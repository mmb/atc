package scheduler_test

import (
	"errors"

	"github.com/concourse/atc/builder/fakebuilder"
	"github.com/concourse/atc/config"
	"github.com/concourse/atc/db"
	dbfakes "github.com/concourse/atc/db/fakes"
	. "github.com/concourse/atc/scheduler"
	"github.com/concourse/atc/scheduler/fakes"
	"github.com/concourse/turbine"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Scheduler", func() {
	var (
		schedulerDB *fakes.FakeSchedulerDB
		factory     *fakes.FakeBuildFactory
		builder     *fakebuilder.FakeBuilder
		locker      *fakes.FakeLocker
		tracker     *fakes.FakeBuildTracker

		createdTurbineBuild turbine.Build

		job config.Job

		lock *dbfakes.FakeLock

		scheduler *Scheduler
	)

	BeforeEach(func() {
		schedulerDB = new(fakes.FakeSchedulerDB)
		factory = new(fakes.FakeBuildFactory)
		builder = new(fakebuilder.FakeBuilder)
		locker = new(fakes.FakeLocker)
		tracker = new(fakes.FakeBuildTracker)

		createdTurbineBuild = turbine.Build{
			Config: turbine.Config{
				Run: turbine.RunConfig{Path: "some-build"},
			},
		}

		factory.CreateReturns(createdTurbineBuild, nil)

		scheduler = &Scheduler{
			Logger:  lagertest.NewTestLogger("test"),
			DB:      schedulerDB,
			Locker:  locker,
			Factory: factory,
			Builder: builder,
			Tracker: tracker,
		}

		job = config.Job{
			Name: "some-job",

			Serial: true,

			Inputs: []config.Input{
				{
					Resource: "some-resource",
					Params:   config.Params{"some": "params"},
				},
				{
					Resource: "some-other-resource",
					Params:   config.Params{"some": "params"},
				},
			},
		}

		lock = new(dbfakes.FakeLock)
		locker.AcquireLockReturns(lock, nil)
	})

	Describe("TrackInFlightBuilds", func() {
		var inFlightBuilds []db.Build

		BeforeEach(func() {
			inFlightBuilds = []db.Build{
				{ID: 1},
				{ID: 2},
				{ID: 3},
			}

			schedulerDB.GetAllStartedBuildsReturns(inFlightBuilds, nil)
		})

		It("invokes the tracker with all currently in-flight builds", func() {
			err := scheduler.TrackInFlightBuilds()
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(tracker.TrackBuildCallCount).Should(Equal(3))
			Ω(tracker.TrackBuildArgsForCall(0)).Should(Equal(inFlightBuilds[0]))
			Ω(tracker.TrackBuildArgsForCall(1)).Should(Equal(inFlightBuilds[1]))
			Ω(tracker.TrackBuildArgsForCall(2)).Should(Equal(inFlightBuilds[2]))
		})
	})

	Describe("BuildLatestInputs", func() {
		Context("when no inputs are available", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				schedulerDB.GetLatestInputVersionsReturns(nil, disaster)
			})

			It("returns the error", func() {
				err := scheduler.BuildLatestInputs(job)
				Ω(err).Should(Equal(disaster))
			})

			It("does not trigger a build", func() {
				scheduler.BuildLatestInputs(job)

				Ω(builder.BuildCallCount()).Should(Equal(0))
			})
		})

		Context("when the job has no inputs", func() {
			BeforeEach(func() {
				job.Inputs = []config.Input{}
			})

			It("succeeds", func() {
				err := scheduler.BuildLatestInputs(job)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("does not try to fetch inputs from the database", func() {
				scheduler.BuildLatestInputs(job)

				Ω(schedulerDB.GetLatestInputVersionsCallCount()).Should(BeZero())
			})

			It("does not trigger a build", func() {
				scheduler.BuildLatestInputs(job)

				Ω(builder.BuildCallCount()).Should(Equal(0))
			})
		})

		Context("when inputs are found", func() {
			foundInputs := db.VersionedResources{
				{Name: "some-resource", Version: db.Version{"version": "1"}},
				{Name: "some-other-resource", Version: db.Version{"version": "2"}},
			}

			BeforeEach(func() {
				schedulerDB.GetLatestInputVersionsReturns(foundInputs, nil)
			})

			It("acquires the input's lock before grabbing them from the DB", func() {
				err := scheduler.BuildLatestInputs(job)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(locker.AcquireLockCallCount()).Should(Equal(1))

				lockedInputs := locker.AcquireLockArgsForCall(0)
				Ω(lockedInputs).Should(Equal([]string{"resource: some-resource", "resource: some-other-resource"}))
				Ω(lock.ReleaseCallCount()).Should(Equal(1))
			})

			It("checks if they are already used for a build", func() {
				err := scheduler.BuildLatestInputs(job)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(schedulerDB.GetJobBuildForInputsCallCount()).Should(Equal(1))

				checkedJob, checkedInputs := schedulerDB.GetJobBuildForInputsArgsForCall(0)
				Ω(checkedJob).Should(Equal("some-job"))
				Ω(checkedInputs).Should(Equal(foundInputs))
			})

			Context("and the job has inputs configured not to check", func() {
				BeforeEach(func() {
					trigger := false

					job.Inputs = append(job.Inputs, config.Input{
						Resource: "some-non-checking-resource",
						Trigger:  &trigger,
					})

					foundInputsWithCheck := append(
						foundInputs,
						db.VersionedResource{
							Name:    "some-non-checking-resource",
							Version: db.Version{"version": 3},
						},
					)

					schedulerDB.GetLatestInputVersionsReturns(foundInputsWithCheck, nil)
				})

				It("excludes them from the inputs when checking for a build", func() {
					err := scheduler.BuildLatestInputs(job)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(schedulerDB.GetJobBuildForInputsCallCount()).Should(Equal(1))

					checkedJob, checkedInputs := schedulerDB.GetJobBuildForInputsArgsForCall(0)
					Ω(checkedJob).Should(Equal("some-job"))
					Ω(checkedInputs).Should(Equal(foundInputs))
				})
			})

			Context("and all inputs are configured not to check", func() {
				BeforeEach(func() {
					trigger := false

					for i, input := range job.Inputs {
						noChecking := input
						noChecking.Trigger = &trigger

						job.Inputs[i] = noChecking
					}
				})

				It("does not check for builds for the inputs", func() {
					err := scheduler.BuildLatestInputs(job)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(schedulerDB.GetJobBuildForInputsCallCount()).Should(Equal(0))
				})

				It("does not create a build", func() {
					err := scheduler.BuildLatestInputs(job)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(schedulerDB.CreateJobBuildWithInputsCallCount()).Should(Equal(0))
				})

				It("does not trigger a build", func() {
					err := scheduler.BuildLatestInputs(job)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(builder.BuildCallCount()).Should(Equal(0))
				})
			})

			Context("and they are not used for a build", func() {
				BeforeEach(func() {
					schedulerDB.GetJobBuildForInputsReturns(db.Build{}, errors.New("no build"))
				})

				It("creates a build with the found inputs", func() {
					err := scheduler.BuildLatestInputs(job)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(schedulerDB.CreateJobBuildWithInputsCallCount()).Should(Equal(1))
					buildJob, buildInputs := schedulerDB.CreateJobBuildWithInputsArgsForCall(0)
					Ω(buildJob).Should(Equal("some-job"))
					Ω(buildInputs).Should(Equal(foundInputs))
				})

				Context("when creating the build succeeds", func() {
					BeforeEach(func() {
						schedulerDB.CreateJobBuildWithInputsReturns(db.Build{ID: 128, Name: "42"}, nil)
					})

					Context("and it can be scheduled", func() {
						BeforeEach(func() {
							schedulerDB.ScheduleBuildReturns(true, nil)
						})

						It("triggers a build of the job with the found inputs", func() {
							err := scheduler.BuildLatestInputs(job)
							Ω(err).ShouldNot(HaveOccurred())

							Ω(schedulerDB.ScheduleBuildCallCount()).Should(Equal(1))
							scheduledBuildID, serial := schedulerDB.ScheduleBuildArgsForCall(0)
							Ω(scheduledBuildID).Should(Equal(128))
							Ω(serial).Should(Equal(job.Serial))

							Ω(factory.CreateCallCount()).Should(Equal(1))
							createJob, createInputs := factory.CreateArgsForCall(0)
							Ω(createJob).Should(Equal(job))
							Ω(createInputs).Should(Equal(foundInputs))

							Ω(builder.BuildCallCount()).Should(Equal(1))
							builtBuild, builtTurbineBuild := builder.BuildArgsForCall(0)
							Ω(builtBuild).Should(Equal(db.Build{ID: 128, Name: "42"}))
							Ω(builtTurbineBuild).Should(Equal(createdTurbineBuild))
						})
					})

					Context("when the build cannot be scheduled", func() {
						BeforeEach(func() {
							schedulerDB.ScheduleBuildReturns(false, nil)
						})

						It("does not start a build", func() {
							err := scheduler.BuildLatestInputs(job)
							Ω(err).ShouldNot(HaveOccurred())

							Ω(builder.BuildCallCount()).Should(Equal(0))
						})
					})
				})

				Context("when creating the build fails", func() {
					disaster := errors.New("oh no!")

					BeforeEach(func() {
						schedulerDB.CreateJobBuildWithInputsReturns(db.Build{}, disaster)
					})

					It("returns the error", func() {
						err := scheduler.BuildLatestInputs(job)
						Ω(err).Should(Equal(disaster))
					})

					It("does not start a build", func() {
						scheduler.BuildLatestInputs(job)
						Ω(builder.BuildCallCount()).Should(Equal(0))
					})
				})
			})

			Context("but they are already used for a build", func() {
				BeforeEach(func() {
					schedulerDB.GetJobBuildForInputsReturns(db.Build{ID: 128, Name: "42"}, nil)
				})

				It("does not trigger a build", func() {
					err := scheduler.BuildLatestInputs(job)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(builder.BuildCallCount()).Should(Equal(0))
				})
			})
		})
	})

	Describe("TryNextPendingBuild", func() {
		Context("when a pending build is found", func() {
			pendingInputs := db.VersionedResources{
				{Name: "some-resource", Version: db.Version{"version": "1"}},
				{Name: "some-other-resource", Version: db.Version{"version": "2"}},
			}

			BeforeEach(func() {
				schedulerDB.GetNextPendingBuildReturns(db.Build{ID: 128, Name: "42"}, pendingInputs, nil)
			})

			Context("and it can be scheduled", func() {
				BeforeEach(func() {
					schedulerDB.ScheduleBuildReturns(true, nil)
				})

				It("builds it", func() {
					err := scheduler.TryNextPendingBuild(job)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(schedulerDB.ScheduleBuildCallCount()).Should(Equal(1))
					scheduledBuildID, serial := schedulerDB.ScheduleBuildArgsForCall(0)
					Ω(scheduledBuildID).Should(Equal(128))
					Ω(serial).Should(Equal(job.Serial))

					Ω(factory.CreateCallCount()).Should(Equal(1))
					createJob, createInputs := factory.CreateArgsForCall(0)
					Ω(createJob).Should(Equal(job))
					Ω(createInputs).Should(Equal(pendingInputs))

					Ω(builder.BuildCallCount()).Should(Equal(1))
					builtBuild, builtTurbineBuild := builder.BuildArgsForCall(0)
					Ω(builtBuild).Should(Equal(db.Build{ID: 128, Name: "42"}))
					Ω(builtTurbineBuild).Should(Equal(createdTurbineBuild))
				})
			})

			Context("when the build cannot be scheduled", func() {
				BeforeEach(func() {
					schedulerDB.ScheduleBuildReturns(false, nil)
				})

				It("does not start a build", func() {
					err := scheduler.TryNextPendingBuild(job)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(builder.BuildCallCount()).Should(Equal(0))
				})
			})
		})

		Context("when a pending build is not found", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				schedulerDB.GetNextPendingBuildReturns(db.Build{}, db.VersionedResources{}, disaster)
			})

			It("returns the error", func() {
				err := scheduler.TryNextPendingBuild(job)
				Ω(err).Should(Equal(disaster))
			})

			It("does not start a build", func() {
				scheduler.TryNextPendingBuild(job)
				Ω(builder.BuildCallCount()).Should(Equal(0))
			})
		})
	})

	Describe("TriggerImmediately", func() {
		Context("when the job does not have any dependant inputs", func() {
			It("creates a build without any specific inputs", func() {
				_, err := scheduler.TriggerImmediately(job)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(schedulerDB.GetLatestInputVersionsCallCount()).Should(Equal(0))

				Ω(schedulerDB.CreateJobBuildWithInputsCallCount()).Should(Equal(1))

				jobName, inputs := schedulerDB.CreateJobBuildWithInputsArgsForCall(0)
				Ω(jobName).Should(Equal("some-job"))
				Ω(inputs).Should(BeZero())
			})

			Context("when creating the build succeeds", func() {
				BeforeEach(func() {
					schedulerDB.CreateJobBuildWithInputsReturns(db.Build{ID: 128, Name: "42"}, nil)
				})

				Context("and it can be scheduled", func() {
					BeforeEach(func() {
						schedulerDB.ScheduleBuildReturns(true, nil)
					})

					It("triggers a build of the job with the found inputs", func() {
						build, err := scheduler.TriggerImmediately(job)
						Ω(err).ShouldNot(HaveOccurred())
						Ω(build).Should(Equal(db.Build{ID: 128, Name: "42"}))

						Ω(schedulerDB.ScheduleBuildCallCount()).Should(Equal(1))
						scheduledBuildID, serial := schedulerDB.ScheduleBuildArgsForCall(0)
						Ω(scheduledBuildID).Should(Equal(128))
						Ω(serial).Should(Equal(job.Serial))

						Ω(factory.CreateCallCount()).Should(Equal(1))
						createJob, createInputs := factory.CreateArgsForCall(0)
						Ω(createJob).Should(Equal(job))
						Ω(createInputs).Should(BeZero())

						Ω(builder.BuildCallCount()).Should(Equal(1))
						builtBuild, builtTurbineBuild := builder.BuildArgsForCall(0)
						Ω(builtBuild).Should(Equal(db.Build{ID: 128, Name: "42"}))
						Ω(builtTurbineBuild).Should(Equal(createdTurbineBuild))
					})
				})

				Context("when the build cannot be scheduled", func() {
					BeforeEach(func() {
						schedulerDB.ScheduleBuildReturns(false, nil)
					})

					It("does not start a build", func() {
						_, err := scheduler.TriggerImmediately(job)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(builder.BuildCallCount()).Should(Equal(0))
					})
				})
			})

			Context("when creating the build fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					schedulerDB.CreateJobBuildWithInputsReturns(db.Build{}, disaster)
				})

				It("returns the error", func() {
					_, err := scheduler.TriggerImmediately(job)
					Ω(err).Should(Equal(disaster))
				})

				It("does not start a build", func() {
					scheduler.TriggerImmediately(job)
					Ω(builder.BuildCallCount()).Should(Equal(0))
				})
			})
		})

		Context("when the job has dependant inputs", func() {
			BeforeEach(func() {
				job.Inputs = append(job.Inputs, config.Input{
					Resource: "some-dependant-resource",
					Passed:   []string{"job-a"},
				})
			})

			Context("and they can be satisfied", func() {
				foundInputs := db.VersionedResources{
					{Name: "some-dependant-resource", Version: db.Version{"version": "2"}},
				}

				BeforeEach(func() {
					schedulerDB.GetLatestInputVersionsReturns(foundInputs, nil)
				})

				It("creates a build with the found inputs", func() {
					_, err := scheduler.TriggerImmediately(job)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(schedulerDB.GetLatestInputVersionsCallCount()).Should(Equal(1))
					Ω(schedulerDB.GetLatestInputVersionsArgsForCall(0)).Should(Equal([]config.Input{
						{
							Resource: "some-dependant-resource",
							Passed:   []string{"job-a"},
						},
					}))

					Ω(schedulerDB.CreateJobBuildWithInputsCallCount()).Should(Equal(1))

					jobName, inputs := schedulerDB.CreateJobBuildWithInputsArgsForCall(0)
					Ω(jobName).Should(Equal("some-job"))
					Ω(inputs).Should(Equal(foundInputs))
				})

				Context("when creating the build succeeds", func() {
					BeforeEach(func() {
						schedulerDB.CreateJobBuildWithInputsReturns(db.Build{ID: 128, Name: "42"}, nil)
					})

					Context("and it can be scheduled", func() {
						BeforeEach(func() {
							schedulerDB.ScheduleBuildReturns(true, nil)
						})

						It("triggers a build of the job with the found inputs", func() {
							build, err := scheduler.TriggerImmediately(job)
							Ω(err).ShouldNot(HaveOccurred())
							Ω(build).Should(Equal(db.Build{ID: 128, Name: "42"}))

							Ω(schedulerDB.ScheduleBuildCallCount()).Should(Equal(1))
							scheduledBuildID, serial := schedulerDB.ScheduleBuildArgsForCall(0)
							Ω(scheduledBuildID).Should(Equal(128))
							Ω(serial).Should(Equal(job.Serial))

							Ω(factory.CreateCallCount()).Should(Equal(1))
							createJob, createInputs := factory.CreateArgsForCall(0)
							Ω(createJob).Should(Equal(job))
							Ω(createInputs).Should(Equal(foundInputs))

							Ω(builder.BuildCallCount()).Should(Equal(1))
							builtBuild, builtTurbineBuild := builder.BuildArgsForCall(0)
							Ω(builtBuild).Should(Equal(db.Build{ID: 128, Name: "42"}))
							Ω(builtTurbineBuild).Should(Equal(createdTurbineBuild))
						})
					})
				})

				Context("when the build cannot be scheduled", func() {
					BeforeEach(func() {
						schedulerDB.ScheduleBuildReturns(false, nil)
					})

					It("does not start a build", func() {
						scheduler.TriggerImmediately(job)
						Ω(builder.BuildCallCount()).Should(Equal(0))
					})
				})

				Context("when creating the build fails", func() {
					disaster := errors.New("oh no!")

					BeforeEach(func() {
						schedulerDB.CreateJobBuildWithInputsReturns(db.Build{}, disaster)
					})

					It("returns the error", func() {
						_, err := scheduler.TriggerImmediately(job)
						Ω(err).Should(Equal(disaster))
					})

					It("does not start a build", func() {
						scheduler.TriggerImmediately(job)
						Ω(builder.BuildCallCount()).Should(Equal(0))
					})
				})
			})

			Context("but they cannot be satisfied", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					schedulerDB.GetLatestInputVersionsReturns(nil, disaster)
				})

				It("returns the error", func() {
					_, err := scheduler.TriggerImmediately(job)
					Ω(err).Should(Equal(disaster))
				})

				It("does not create or start a build", func() {
					scheduler.TriggerImmediately(job)

					Ω(schedulerDB.CreateJobBuildWithInputsCallCount()).Should(Equal(0))

					Ω(builder.BuildCallCount()).Should(Equal(0))
				})
			})
		})
	})
})
