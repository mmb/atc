package buildstarter_test

import (
	"errors"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/engine/enginefakes"
	"github.com/concourse/atc/scheduler/buildstarter"
	"github.com/concourse/atc/scheduler/buildstarter/buildstarterfakes"
	"github.com/concourse/atc/scheduler/buildstarter/maxinflight/maxinflightfakes"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("I'm the BuildStarter", func() {
	var (
		fakeDB                 *buildstarterfakes.FakeBuildStarterDB
		fakeBuildsDB           *buildstarterfakes.FakeBuildStarterBuildsDB
		fakeMaxInFlightUpdater *maxinflightfakes.FakeMaxInFlightUpdater
		fakeFactory            *buildstarterfakes.FakeBuildFactory
		fakeEngine             *enginefakes.FakeEngine

		buildStarter buildstarter.BuildStarter

		disaster error
	)

	BeforeEach(func() {
		fakeDB = new(buildstarterfakes.FakeBuildStarterDB)
		fakeBuildsDB = new(buildstarterfakes.FakeBuildStarterBuildsDB)
		fakeMaxInFlightUpdater = new(maxinflightfakes.FakeMaxInFlightUpdater)
		fakeFactory = new(buildstarterfakes.FakeBuildFactory)
		fakeEngine = new(enginefakes.FakeEngine)

		buildStarter = buildstarter.NewBuildStarter(fakeDB, fakeBuildsDB, fakeMaxInFlightUpdater, fakeFactory, fakeEngine)

		disaster = errors.New("bad thing")
	})

	Describe("TryStartAllPendingBuilds", func() {
		var tryStartErr error

		JustBeforeEach(func() {
			tryStartErr = buildStarter.TryStartAllPendingBuilds(
				lagertest.NewTestLogger("test"),
				atc.JobConfig{Name: "some-job"},
				atc.ResourceConfigs{{Name: "some-resource"}},
				atc.ResourceTypes{{Name: "some-resource-type"}})
		})

		Context("when the stars align", func() {
			BeforeEach(func() {
				fakeMaxInFlightUpdater.UpdateMaxInFlightReachedReturns(false, nil)
				fakeDB.GetNextBuildInputsReturns([]db.BuildInput{{Name: "some-input"}}, true, nil)
				fakeDB.IsPausedReturns(false, nil)
				fakeDB.GetJobReturns(db.SavedJob{Paused: false}, nil)
			})

			Context("when getting the next pending build fails", func() {
				BeforeEach(func() {
					fakeDB.GetNextPendingBuildReturns(db.Build{}, false, disaster)
				})

				It("returns the error", func() {
					Expect(tryStartErr).To(Equal(disaster))
				})

				It("got the pending build for the right job", func() {
					Expect(fakeDB.GetNextPendingBuildCallCount()).To(Equal(1))
					Expect(fakeDB.GetNextPendingBuildArgsForCall(0)).To(Equal("some-job"))
				})
			})

			Context("when there is no pending build", func() {
				BeforeEach(func() {
					fakeDB.GetNextPendingBuildReturns(db.Build{}, false, nil)
				})

				It("doesn't return an error", func() {
					Expect(tryStartErr).NotTo(HaveOccurred())
				})
			})

			Context("when there is one pending build", func() {
				var pendingBuild db.Build

				BeforeEach(func() {
					pendingBuild = db.Build{ID: 47, PipelineID: 52}
					fakeDB.GetNextPendingBuildStub = func(string) (db.Build, bool, error) {
						if fakeDB.GetNextBuildInputsCallCount() == 0 {
							return pendingBuild, true, nil
						}
						return db.Build{}, false, nil
					}
				})

				Context("when marking the build as scheduled fails", func() {
					BeforeEach(func() {
						fakeDB.UpdateBuildToScheduledReturns(false, disaster)
					})

					It("returns the error", func() {
						Expect(tryStartErr).To(Equal(disaster))
					})

					It("marked the right build as scheduled", func() {
						Expect(fakeDB.UpdateBuildToScheduledCallCount()).To(Equal(1))
						Expect(fakeDB.UpdateBuildToScheduledArgsForCall(0)).To(Equal(pendingBuild.ID))
					})
				})

				Context("when someone else already scheduled the build", func() {
					BeforeEach(func() {
						fakeDB.UpdateBuildToScheduledReturns(false, nil)
					})

					It("doesn't return an error", func() {
						Expect(tryStartErr).NotTo(HaveOccurred())
					})

					It("doesn't try to use inputs for build", func() {
						Expect(fakeDB.UseInputsForBuildCallCount()).To(BeZero())
					})
				})

				Context("when marking the build as scheduled succeeds", func() {
					BeforeEach(func() {
						fakeDB.UpdateBuildToScheduledReturns(true, nil)
					})

					Context("when using inputs for build fails", func() {
						BeforeEach(func() {
							fakeDB.UseInputsForBuildReturns(disaster)
						})

						It("returns the error", func() {
							Expect(tryStartErr).To(Equal(disaster))
						})

						It("used the right inputs for the right build", func() {
							Expect(fakeDB.UseInputsForBuildCallCount()).To(Equal(1))
							actualBuildID, actualInputs := fakeDB.UseInputsForBuildArgsForCall(0)
							Expect(actualBuildID).To(Equal(pendingBuild.ID))
							Expect(actualInputs).To(Equal([]db.BuildInput{{Name: "some-input"}}))
						})
					})

					Context("when using inputs for build succeeds", func() {
						BeforeEach(func() {
							fakeDB.UseInputsForBuildReturns(nil)
						})

						Context("when creating the build plan fails", func() {
							BeforeEach(func() {
								fakeFactory.CreateReturns(atc.Plan{}, disaster)
							})

							It("created the build plan with the right config", func() {
								Expect(fakeFactory.CreateCallCount()).To(Equal(1))
								actualJobConfig, actualResourceConfigs, actualResourceTypes, actualBuildInputs := fakeFactory.CreateArgsForCall(0)
								Expect(actualJobConfig).To(Equal(atc.JobConfig{Name: "some-job"}))
								Expect(actualResourceConfigs).To(Equal(atc.ResourceConfigs{{Name: "some-resource"}}))
								Expect(actualResourceTypes).To(Equal(atc.ResourceTypes{{Name: "some-resource-type"}}))
								Expect(actualBuildInputs).To(Equal([]db.BuildInput{{Name: "some-input"}}))
							})

							Context("when marking the build as errored fails", func() {
								BeforeEach(func() {
									fakeBuildsDB.FinishBuildReturns(disaster)
								})

								It("doesn't return an error", func() {
									Expect(tryStartErr).NotTo(HaveOccurred())
								})

								It("marked the right build as errored", func() {
									Expect(fakeBuildsDB.FinishBuildCallCount()).To(Equal(1))
									actualBuildID, actualPipelineID, actualStatus := fakeBuildsDB.FinishBuildArgsForCall(0)
									Expect(actualBuildID).To(Equal(pendingBuild.ID))
									Expect(actualPipelineID).To(Equal(pendingBuild.PipelineID))
									Expect(actualStatus).To(Equal(db.StatusErrored))
								})
							})

							Context("when marking the build as errored succeeds", func() {
								BeforeEach(func() {
									fakeBuildsDB.FinishBuildReturns(nil)
								})

								It("doesn't return an error", func() {
									Expect(tryStartErr).NotTo(HaveOccurred())
								})
							})
						})

						Context("when creating the build plan succeeds", func() {
							BeforeEach(func() {
								fakeFactory.CreateReturns(atc.Plan{Task: &atc.TaskPlan{ConfigPath: "some-task.yml"}}, nil)
							})

							Context("when creating the engine build fails", func() {
								BeforeEach(func() {
									fakeEngine.CreateBuildReturns(nil, disaster)
								})

								It("doesn't return an error", func() {
									Expect(tryStartErr).NotTo(HaveOccurred())
								})

								It("created the engine build with the right build and plan", func() {
									Expect(fakeEngine.CreateBuildCallCount()).To(Equal(1))
									_, actualBuild, actualPlan := fakeEngine.CreateBuildArgsForCall(0)
									Expect(actualBuild).To(Equal(pendingBuild))
									Expect(actualPlan).To(Equal(atc.Plan{Task: &atc.TaskPlan{ConfigPath: "some-task.yml"}}))
								})
							})

							Context("when creating the engine build succeeds", func() {
								var engineBuild *enginefakes.FakeBuild

								BeforeEach(func() {
									engineBuild = new(enginefakes.FakeBuild)
									fakeEngine.CreateBuildReturns(engineBuild, nil)
								})

								It("doesn't return an error", func() {
									Expect(tryStartErr).NotTo(HaveOccurred())
								})

								It("starts the engine build (asynchronously)", func() {
									Eventually(engineBuild.ResumeCallCount).Should(Equal(1))
								})
							})
						})
					})
				})
			})
		})
	})
})
