package scheduler_test

import (
	"errors"
	"time"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/dbfakes"
	. "github.com/concourse/atc/scheduler"
	"github.com/concourse/atc/scheduler/buildstarter/buildstarterfakes"
	"github.com/concourse/atc/scheduler/inputmapper/inputmapperfakes"
	"github.com/concourse/atc/scheduler/schedulerfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = FDescribe("Scheduler", func() {
	var (
		fakeDB           *schedulerfakes.FakeSchedulerDB
		fakeInputMapper  *inputmapperfakes.FakeInputMapper
		fakeBuildStarter *buildstarterfakes.FakeBuildStarter

		scheduler *Scheduler

		disaster error
	)

	BeforeEach(func() {
		fakeDB = new(schedulerfakes.FakeSchedulerDB)
		fakeInputMapper = new(inputmapperfakes.FakeInputMapper)
		fakeBuildStarter = new(buildstarterfakes.FakeBuildStarter)

		scheduler = &Scheduler{
			DB:           fakeDB,
			InputMapper:  fakeInputMapper,
			BuildStarter: fakeBuildStarter,
		}

		disaster = errors.New("bad thing")
	})

	Describe("TriggerImmediately", func() {
		var (
			triggeredBuild db.Build
			triggerErr     error
		)

		JustBeforeEach(func() {
			var waiter Waiter
			triggeredBuild, waiter, triggerErr = scheduler.TriggerImmediately(
				lagertest.NewTestLogger("test"),
				atc.JobConfig{Name: "some-job"},
				atc.ResourceConfigs{{Name: "some-resource"}},
				atc.ResourceTypes{{Name: "some-resource-type"}})
			if waiter != nil {
				waiter.Wait()
			}
		})

		Context("when getting the lease errors", func() {
			BeforeEach(func() {
				fakeDB.LeaseResourceCheckingForJobReturns(nil, false, disaster)
			})

			It("returns the error", func() {
				Expect(triggerErr).To(Equal(disaster))
			})

			It("asked for the lease for the right job", func() {
				Expect(fakeDB.LeaseResourceCheckingForJobCallCount()).To(Equal(1))
				_, actualJobName, actualInterval := fakeDB.LeaseResourceCheckingForJobArgsForCall(0)
				Expect(actualJobName).To(Equal("some-job"))
				Expect(actualInterval).To(Equal(5 * time.Minute))
			})
		})

		Context("when someone else is holding the lease", func() {
			BeforeEach(func() {
				fakeDB.LeaseResourceCheckingForJobReturns(nil, false, nil)
			})

			Context("when creating the build fails", func() {
				BeforeEach(func() {
					fakeDB.CreateJobBuildReturns(db.Build{}, disaster)
				})

				It("returns the error", func() {
					Expect(triggerErr).To(Equal(disaster))
				})

				It("created a build for the right job", func() {
					Expect(fakeDB.CreateJobBuildCallCount()).To(Equal(1))
					Expect(fakeDB.CreateJobBuildArgsForCall(0)).To(Equal("some-job"))
				})
			})

			Context("when creating the build succeeds", func() {
				var createdBuild db.Build

				BeforeEach(func() {
					createdBuild = db.Build{ID: 99}
					fakeDB.CreateJobBuildStub = func(jobName string) (db.Build, error) {
						defer GinkgoRecover()
						Expect(fakeBuildStarter.TryStartAllPendingBuildsCallCount()).To(BeZero())
						return createdBuild, nil
					}
				})

				Context("when starting all pending builds fails", func() {
					BeforeEach(func() {
						fakeBuildStarter.TryStartAllPendingBuildsReturns(disaster)
					})

					It("tried to start all pending builds after creating the build", func() {
						Expect(fakeBuildStarter.TryStartAllPendingBuildsCallCount()).To(Equal(1))
						_, actualJob, actualResources, actualResourceTypes := fakeBuildStarter.TryStartAllPendingBuildsArgsForCall(0)
						Expect(actualJob).To(Equal(atc.JobConfig{Name: "some-job"}))
						Expect(actualResources).To(Equal(atc.ResourceConfigs{{Name: "some-resource"}}))
						Expect(actualResourceTypes).To(Equal(atc.ResourceTypes{{Name: "some-resource-type"}}))
					})

					It("returns the build", func() {
						Expect(triggerErr).NotTo(HaveOccurred())
						Expect(triggeredBuild).To(Equal(createdBuild))
					})
				})

				Context("when starting all pending builds succeeds", func() {
					BeforeEach(func() {
						fakeBuildStarter.TryStartAllPendingBuildsReturns(nil)
					})

					It("returns the build", func() {
						Expect(triggerErr).NotTo(HaveOccurred())
						Expect(triggeredBuild).To(Equal(createdBuild))
					})
				})
			})
		})

		Context("when it gets the lease", func() {
			var lease *dbfakes.FakeLease

			BeforeEach(func() {
				lease = new(dbfakes.FakeLease)
				fakeDB.LeaseResourceCheckingForJobReturns(lease, true, nil)
			})

			Context("when creating the build fails", func() {
				BeforeEach(func() {
					fakeDB.CreateJobBuildReturns(db.Build{}, disaster)
				})

				It("returns the error", func() {
					Expect(triggerErr).To(Equal(disaster))
				})

				It("breaks the lease", func() {
					Expect(lease.BreakCallCount()).To(Equal(1))
				})
			})
		})
	})
})
