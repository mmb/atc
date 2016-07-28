package scheduler_test

import (
	. "github.com/onsi/ginkgo"
)

var _ = FDescribe("Scheduler", func() {
	// var (
	// 	fakeSchedulerDB *schedulerfakes.FakeSchedulerDB
	// 	fakeBuildsDB    *schedulerfakes.FakeSchedulerBuildsDB
	// 	factory         *schedulerfakes.FakeBuildFactory
	// 	fakeEngine      *enginefakes.FakeEngine
	// 	fakeScanner     *schedulerfakes.FakeScanner
	//
	// 	lease *dbfakes.FakeLease
	//
	// 	job           atc.JobConfig
	// 	resources     atc.ResourceConfigs
	// 	resourceTypes atc.ResourceTypes
	//
	// 	scheduler *Scheduler
	//
	// 	logger *lagertest.TestLogger
	//
	// 	disaster error
	// )
	//
	// BeforeEach(func() {
	// 	fakeSchedulerDB = new(schedulerfakes.FakeSchedulerDB)
	// 	fakeBuildsDB = new(schedulerfakes.FakeSchedulerBuildsDB)
	// 	factory = new(schedulerfakes.FakeBuildFactory)
	// 	fakeEngine = new(enginefakes.FakeEngine)
	// 	fakeScanner = new(schedulerfakes.FakeScanner)
	//
	// 	scheduler = &Scheduler{
	// 		DB:       fakeSchedulerDB,
	// 		BuildsDB: fakeBuildsDB,
	// 		Factory:  factory,
	// 		Engine:   fakeEngine,
	// 		Scanner:  fakeScanner,
	// 	}
	//
	// 	logger = lagertest.NewTestLogger("test")
	//
	// 	job = atc.JobConfig{
	// 		Name: "some-job",
	//
	// 		Serial: true,
	//
	// 		Plan: atc.PlanSequence{
	// 			{
	// 				Get:      "some-input",
	// 				Resource: "some-resource",
	// 				Params:   atc.Params{"some": "params"},
	// 				Trigger:  true,
	// 			},
	// 			{
	// 				Get:      "some-other-input",
	// 				Resource: "some-other-resource",
	// 				Params:   atc.Params{"some": "other-params"},
	// 				Trigger:  true,
	// 			},
	// 		},
	// 	}
	//
	// 	resources = atc.ResourceConfigs{
	// 		{
	// 			Name:   "some-resource",
	// 			Type:   "git",
	// 			Source: atc.Source{"uri": "git://some-resource"},
	// 		},
	// 		{
	// 			Name:   "some-other-resource",
	// 			Type:   "git",
	// 			Source: atc.Source{"uri": "git://some-other-resource"},
	// 		},
	// 		{
	// 			Name:   "some-dependant-resource",
	// 			Type:   "git",
	// 			Source: atc.Source{"uri": "git://some-dependant-resource"},
	// 		},
	// 		{
	// 			Name:   "some-output-resource",
	// 			Type:   "git",
	// 			Source: atc.Source{"uri": "git://some-output-resource"},
	// 		},
	// 		{
	// 			Name:   "some-resource-with-longer-name",
	// 			Type:   "git",
	// 			Source: atc.Source{"uri": "git://some-resource-with-longer-name"},
	// 		},
	// 		{
	// 			Name:   "some-named-resource",
	// 			Type:   "git",
	// 			Source: atc.Source{"uri": "git://some-named-resource"},
	// 		},
	// 	}
	//
	// 	resourceTypes = atc.ResourceTypes{
	// 		{
	// 			Name:   "some-custom-resource",
	// 			Type:   "custom-type",
	// 			Source: atc.Source{"custom": "source"},
	// 		},
	// 	}
	//
	// 	lease = new(dbfakes.FakeLease)
	// 	fakeSchedulerDB.LeaseSchedulingReturns(lease, true, nil)
	//
	// 	disaster = errors.New("bad thing")
	// })
	//
	// Describe("TriggerImmediately", func() {
	// 	Context("when getting the lease succeeds", func() {
	// 		var lease *dbfakes.FakeLease
	//
	// 		BeforeEach(func() {
	// 			lease = new(dbfakes.FakeLease)
	// 			fakeSchedulerDB.LeaseResourceCheckingForJobReturns(lease, true, nil)
	// 		})
	//
	// 		Context("when creating the build succeeds", func() {
	// 			var createdBuild db.Build
	// 			BeforeEach(func() {
	// 				createdBuild = db.Build{ID: 99}
	// 				fakeSchedulerDB.CreateJobBuildReturns(createdBuild, nil)
	// 			})
	//
	// 			It("returns the build", func() {
	// 				actualBuild, _, err := scheduler.TriggerImmediately(logger, job, resources, resourceTypes)
	// 				Expect(err).NotTo(HaveOccurred())
	// 				Expect(actualBuild).To(Equal(createdBuild))
	//
	// 				Expect(fakeSchedulerDB.CreateJobBuildCallCount()).To(Equal(1))
	// 				jobName := fakeSchedulerDB.CreateJobBuildArgsForCall(0)
	// 				Expect(jobName).To(Equal("some-job"))
	// 			})
	//
	// 			It("tries to start all pending builds", func() {})
	// 		})
	//
	// 		Context("when creating the build fails", func() {
	// 			BeforeEach(func() {
	// 				fakeSchedulerDB.CreateJobBuildReturns(db.Build{}, disaster)
	// 			})
	//
	// 			It("returns the error and breaks the lease", func() {
	// 				_, _, err := scheduler.TriggerImmediately(logger, job, resources, resourceTypes)
	// 				Expect(err).To(Equal(disaster))
	// 				Expect(lease.BreakCallCount()).To(Equal(1))
	// 			})
	// 		})
	// 	})
	//
	// 	Context("when getting lease fails", func() {
	// 		BeforeEach(func() {
	// 			fakeSchedulerDB.LeaseResourceCheckingForJobReturns(nil, false, disaster)
	// 		})
	//
	// 		It("returns an error", func() {
	// 			_, _, err := scheduler.TriggerImmediately(logger, job, resources, resourceTypes)
	// 			Expect(err).To(Equal(disaster))
	// 		})
	// 	})
	//
	// 	Context("when did not get a lease", func() {
	// 		BeforeEach(func() {
	// 			fakeSchedulerDB.LeaseResourceCheckingForJobReturns(nil, false, nil)
	// 		})
	//
	// 		Context("when creating the build succeeds", func() {
	// 			var createdBuild db.Build
	// 			BeforeEach(func() {
	// 				createdBuild = db.Build{ID: 99}
	// 				fakeSchedulerDB.CreateJobBuildReturns(createdBuild, nil)
	// 			})
	//
	// 			It("returns the build", func() {
	// 				actualBuild, _, err := scheduler.TriggerImmediately(logger, job, resources, resourceTypes)
	// 				Expect(err).NotTo(HaveOccurred())
	// 				Expect(actualBuild).To(Equal(createdBuild))
	//
	// 				Expect(fakeSchedulerDB.CreateJobBuildCallCount()).To(Equal(1))
	// 				jobName := fakeSchedulerDB.CreateJobBuildArgsForCall(0)
	// 				Expect(jobName).To(Equal("some-job"))
	// 			})
	//
	// 			It("tries to start all pending builds", func() {})
	// 		})
	//
	// 		Context("when creating the build fails", func() {
	// 			BeforeEach(func() {
	// 				fakeSchedulerDB.CreateJobBuildReturns(db.Build{}, disaster)
	// 			})
	//
	// 			It("returns the error and does not break the lease", func() {
	// 				_, _, err := scheduler.TriggerImmediately(logger, job, resources, resourceTypes)
	// 				Expect(err).To(Equal(disaster))
	// 			})
	// 		})
	// 	})
	// })

})
