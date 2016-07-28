package scheduler_test

import (
	"errors"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/engine/enginefakes"
	. "github.com/concourse/atc/scheduler"
	"github.com/concourse/atc/scheduler/schedulerfakes"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Buildstarter", func() {
	var (
		fakeDB       *schedulerfakes.FakeBuildStarterDB
		fakeBuildsDB *schedulerfakes.FakeBuildStarterBuildsDB
		factory      *schedulerfakes.FakeBuildFactory
		fakeEngine   *enginefakes.FakeEngine

		job           atc.JobConfig
		resources     atc.ResourceConfigs
		resourceTypes atc.ResourceTypes
		engineBuild   *enginefakes.FakeBuild

		buildStarter *BuildStarter

		logger *lagertest.TestLogger

		disaster error
	)

	BeforeEach(func() {
		fakeDB = new(schedulerfakes.FakeBuildStarterDB)
		fakeBuildsDB = new(schedulerfakes.FakeBuildStarterBuildsDB)
		factory = new(schedulerfakes.FakeBuildFactory)
		fakeEngine = new(enginefakes.FakeEngine)

		buildStarter = &BuildStarter{
			DB:       fakeDB,
			BuildsDB: fakeBuildsDB,
			Factory:  factory,
			Engine:   fakeEngine,
		}

		logger = lagertest.NewTestLogger("test")

		job = atc.JobConfig{
			Name: "some-job",

			Serial: true,

			Plan: atc.PlanSequence{
				{
					Get:      "some-input",
					Resource: "some-resource",
					Params:   atc.Params{"some": "params"},
					Trigger:  true,
				},
				{
					Get:      "some-other-input",
					Resource: "some-other-resource",
					Params:   atc.Params{"some": "other-params"},
					Trigger:  true,
				},
			},
		}

		resources = atc.ResourceConfigs{
			{
				Name:   "some-resource",
				Type:   "git",
				Source: atc.Source{"uri": "git://some-resource"},
			},
			{
				Name:   "some-other-resource",
				Type:   "git",
				Source: atc.Source{"uri": "git://some-other-resource"},
			},
			{
				Name:   "some-dependant-resource",
				Type:   "git",
				Source: atc.Source{"uri": "git://some-dependant-resource"},
			},
			{
				Name:   "some-output-resource",
				Type:   "git",
				Source: atc.Source{"uri": "git://some-output-resource"},
			},
			{
				Name:   "some-resource-with-longer-name",
				Type:   "git",
				Source: atc.Source{"uri": "git://some-resource-with-longer-name"},
			},
			{
				Name:   "some-named-resource",
				Type:   "git",
				Source: atc.Source{"uri": "git://some-named-resource"},
			},
		}

		resourceTypes = atc.ResourceTypes{
			{
				Name:   "some-custom-resource",
				Type:   "custom-type",
				Source: atc.Source{"custom": "source"},
			},
		}

		disaster = errors.New("bad thing")
		engineBuild = new(enginefakes.FakeBuild)
		fakeEngine.CreateBuildReturns(engineBuild, nil)
	})

	Describe("TryStartAllPendingBuilds", func() {
		Context("when there is one pending build", func() {
			var pendingBuild db.Build
			var tryStartErr error

			BeforeEach(func() {
				pendingBuild = db.Build{ID: 47, PipelineID: 52}
				fakeDB.GetNextPendingBuildStub = func(string) (db.Build, bool, error) {
					if fakeDB.GetNextBuildInputsCallCount() == 0 {
						return pendingBuild, true, nil
					}
					return db.Build{}, false, nil
				}
			})

			JustBeforeEach(func() {
				tryStartErr = buildStarter.TryStartAllPendingBuilds(logger, job, resources, resourceTypes)
			})

			Context("when max in flight is not reached", func() {
				BeforeEach(func() {
					job.Serial = false
				})

				It("updates max in flight reached", func() {
					Expect(fakeDB.SetMaxInFlightReachedCallCount()).To(Equal(1))
					jobName, maxInFlightReached := fakeDB.SetMaxInFlightReachedArgsForCall(0)
					Expect(jobName).To(Equal("some-job"))
					Expect(maxInFlightReached).To(BeFalse())
				})

				It("gets the pending builds for the right job", func() {
					Expect(fakeDB.GetNextBuildInputsCallCount()).To(Equal(1))
					Expect(fakeDB.GetNextBuildInputsArgsForCall(0)).To(Equal("some-job"))
				})

				Context("when there are next build inputs", func() {
					var nextBuildInputs []db.BuildInput
					BeforeEach(func() {
						nextBuildInputs = []db.BuildInput{{Name: "some-input"}}
						fakeDB.GetNextBuildInputsReturns(nextBuildInputs, true, nil)
					})

					Context("when the pipeline is not paused", func() {
						BeforeEach(func() {
							fakeDB.IsPausedReturns(false, nil)
						})

						Context("when job is not paused", func() {
							BeforeEach(func() {
								fakeDB.GetJobReturns(db.SavedJob{ID: 2, Paused: false}, nil)
							})

							It("marks the right build as scheduled", func() {
								Expect(fakeDB.UpdateBuildToScheduledCallCount()).To(Equal(1))
								Expect(fakeDB.UpdateBuildToScheduledArgsForCall(0)).To(Equal(pendingBuild.ID))
							})

							Context("when marking build as scheduled succeeds", func() {
								BeforeEach(func() {
									fakeDB.UpdateBuildToScheduledReturns(true, nil)
								})

								It("uses the right build inputs", func() {
									Expect(fakeDB.UseInputsForBuildCallCount()).To(Equal(1))
									actualBuildID, actualInputs := fakeDB.UseInputsForBuildArgsForCall(0)
									Expect(actualBuildID).To(Equal(pendingBuild.ID))
									Expect(actualInputs).To(ConsistOf(nextBuildInputs))
								})

								Context("when using build inputs succeeds", func() {
									BeforeEach(func() {
										fakeDB.UseInputsForBuildReturns(nil)
									})

									It("creates build plan with the right config", func() {
										Expect(factory.CreateCallCount()).To(Equal(1))
										actualJobConfig, actualResourceConfigs, actualResourceTypes, actualBuildInputs := factory.CreateArgsForCall(0)
										Expect(actualJobConfig).To(Equal(job))
										Expect(actualResourceConfigs).To(Equal(resources))
										Expect(actualResourceTypes).To(Equal(resourceTypes))
										Expect(actualBuildInputs).To(Equal(nextBuildInputs))
									})

									Context("when creating build plan succeeds", func() {
										var createdPlan atc.Plan
										BeforeEach(func() {
											createdPlan = atc.Plan{
												Task: &atc.TaskPlan{
													Config: &atc.TaskConfig{
														Run: atc.TaskRunConfig{Path: "some-task"},
													},
												},
											}

											factory.CreateReturns(createdPlan, nil)
										})

										It("created engine build with right plan", func() {
											Expect(fakeEngine.CreateBuildCallCount()).To(Equal(1))
											_, actualPendingBuild, actualPlan := fakeEngine.CreateBuildArgsForCall(0)
											Expect(actualPendingBuild).To(Equal(pendingBuild))
											Expect(actualPlan).To(Equal(createdPlan))
										})

										It("resumes the created build", func() {
											Eventually(engineBuild.ResumeCallCount).Should(Equal(1))
										})

										It("doesn't return an error", func() {
											Expect(tryStartErr).NotTo(HaveOccurred())
										})

										Context("when creating engine build fails", func() {
											BeforeEach(func() {
												fakeEngine.CreateBuildReturns(nil, disaster)
											})

											It("doesn't return an error", func() {
												Expect(tryStartErr).NotTo(HaveOccurred())
											})
										})
									})

									Context("when creating the build plan fails", func() {
										BeforeEach(func() {
											factory.CreateReturns(atc.Plan{}, disaster)
										})

										It("doesn't return an error", func() {
											Expect(tryStartErr).NotTo(HaveOccurred())
										})

										It("finishes the build", func() {
											Expect(fakeBuildsDB.FinishBuildCallCount()).To(Equal(1))
											buildID, pipelineID, status := fakeBuildsDB.FinishBuildArgsForCall(0)
											Expect(buildID).To(Equal(pendingBuild.ID))
											Expect(pipelineID).To(Equal(pendingBuild.PipelineID))
											Expect(status).To(Equal(db.StatusErrored))
										})
									})
								})

								Context("when using build inputs fails", func() {
									BeforeEach(func() {
										fakeDB.UseInputsForBuildReturns(disaster)
									})

									It("returns the error", func() {
										Expect(tryStartErr).To(Equal(disaster))
									})

									It("does not create the plan", func() {
										Expect(factory.CreateCallCount()).To(BeZero())
									})
								})
							})

							Context("when marking build as scheduled fails", func() {
								BeforeEach(func() {
									fakeDB.UpdateBuildToScheduledReturns(false, disaster)
								})

								It("does not use build inputs", func() {
									Expect(fakeDB.UseInputsForBuildCallCount()).To(BeZero())
								})
							})
						})

						Context("when job is paused", func() {
							BeforeEach(func() {
								fakeDB.GetJobReturns(db.SavedJob{ID: 2, Paused: true}, nil)
							})

							It("doesn't return an error", func() {
								Expect(tryStartErr).NotTo(HaveOccurred())
							})

							It("doesn't mark the build as scheduled", func() {
								Expect(fakeDB.UpdateBuildToScheduledCallCount()).To(BeZero())
							})
						})

						Context("when getting job fails", func() {
							BeforeEach(func() {
								fakeDB.GetJobReturns(db.SavedJob{}, disaster)
							})

							It("returns the error", func() {
								Expect(tryStartErr).To(Equal(disaster))
							})

							It("doesn't mark the build as scheduled", func() {
								Expect(fakeDB.UpdateBuildToScheduledCallCount()).To(BeZero())
							})
						})
					})

					Context("when the pipeline is paused", func() {
						BeforeEach(func() {
							fakeDB.IsPausedReturns(true, nil)
						})

						It("doesn't return an error", func() {
							Expect(tryStartErr).NotTo(HaveOccurred())
						})

						It("doesn't mark the build as scheduled", func() {
							Expect(fakeDB.UpdateBuildToScheduledCallCount()).To(BeZero())
						})
					})
				})

				Context("when there are no next build inputs", func() {
					BeforeEach(func() {
						fakeDB.GetNextBuildInputsReturns(nil, false, nil)
					})

					It("doesn't return an error", func() {
						Expect(tryStartErr).NotTo(HaveOccurred())
					})

					It("doesn't mark the build as scheduled", func() {
						Expect(fakeDB.UpdateBuildToScheduledCallCount()).To(BeZero())
					})
				})

				Context("when getting next build inputs fails", func() {
					BeforeEach(func() {
						fakeDB.GetNextBuildInputsReturns(nil, false, disaster)
					})

					It("returns the error", func() {
						Expect(tryStartErr).To(Equal(disaster))
					})

					It("doesn't mark the build as scheduled", func() {
						Expect(fakeDB.UpdateBuildToScheduledCallCount()).To(BeZero())
					})
				})
			})

			Context("when max in flight is reached", func() {
				BeforeEach(func() {
					fakeDB.GetRunningBuildsBySerialGroupReturns([]db.Build{{}}, nil)
				})

				It("doesn't return an error", func() {
					Expect(tryStartErr).NotTo(HaveOccurred())
				})

				It("updates max in flight reached", func() {
					Expect(fakeDB.SetMaxInFlightReachedCallCount()).To(Equal(1))
					jobName, maxInFlightReached := fakeDB.SetMaxInFlightReachedArgsForCall(0)
					Expect(jobName).To(Equal("some-job"))
					Expect(maxInFlightReached).To(BeTrue())
				})

				It("doesn't mark the build as scheduled", func() {
					Expect(fakeDB.UpdateBuildToScheduledCallCount()).To(BeZero())
				})
			})

			Context("when checking if max in flight is reached fails", func() {
				BeforeEach(func() {
					fakeDB.GetRunningBuildsBySerialGroupReturns(nil, disaster)
				})

				It("returns the error", func() {
					Expect(tryStartErr).To(Equal(disaster))
				})

				It("doesn't mark the build as scheduled", func() {
					Expect(fakeDB.UpdateBuildToScheduledCallCount()).To(BeZero())
				})

				It("doesn't update max in flight reached", func() {
					Expect(fakeDB.SetMaxInFlightReachedCallCount()).To(BeZero())
				})
			})
		})

		// Context("when getting pending build fails", func() {
		// 	BeforeEach(func() {
		// 		fakeDB.GetNextPendingBuildReturns(db.Build{}, false, disaster)
		// 	})
		//
		// 	It("returns an error", func() {
		// 		_, _, err := scheduler.TriggerImmediately(logger, job, resources, resourceTypes)
		// 		Expect(err).To(Equal(disaster))
		// 	})
		// })
		//
		// Context("when there is no pending build", func() {
		// 	BeforeEach(func() {
		// 		fakeDB.GetNextPendingBuildReturns(db.Build{}, false, nil)
		// 	})
		//
		// 	It("does not return an error", func() {
		// 		_, _, err := scheduler.TriggerImmediately(logger, job, resources, resourceTypes)
		// 		Expect(err).NotTo(HaveOccurred())
		// 	})
		// })
	})
})
