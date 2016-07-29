package scheduler

import (
	"sync"
	"time"

	"github.com/concourse/atc"
	"github.com/concourse/atc/config"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/algorithm"
	"github.com/concourse/atc/metric"
	"github.com/concourse/atc/scheduler/buildstarter"
	"github.com/concourse/atc/scheduler/inputmapper"
	"github.com/pivotal-golang/lager"
)

const ResourceCheckingForJobTimeout = 5 * time.Minute

type Scheduler struct {
	DB           SchedulerDB
	Scanner      Scanner
	BuildStarter buildstarter.BuildStarter
	InputMapper  inputmapper.InputMapper
}

//go:generate counterfeiter . SchedulerDB

type SchedulerDB interface {
	LeaseScheduling(lager.Logger, time.Duration) (db.Lease, bool, error)
	LoadVersionsDB() (*algorithm.VersionsDB, error)
	GetPipelineName() string
	GetConfig() (atc.Config, db.ConfigVersion, bool, error)
	CreateJobBuild(job string) (db.Build, error)
	EnsurePendingBuildExists(jobName string) error
	LeaseResourceCheckingForJob(logger lager.Logger, job string, interval time.Duration) (db.Lease, bool, error)
}

//go:generate counterfeiter . Scanner

type Scanner interface {
	Scan(lager.Logger, string) error
}

func (s *Scheduler) Schedule(logger lager.Logger, interval time.Duration) error {
	logger = logger.Session("schedule")

	schedulingLease, leased, err := s.DB.LeaseScheduling(logger, interval)
	if err != nil {
		logger.Error("failed-to-acquire-scheduling-lease", err)
		return nil
	}

	if !leased {
		return nil
	}

	defer schedulingLease.Break()

	pipelineConfig, _, found, err := s.DB.GetConfig()
	if err != nil {
		logger.Error("failed-to-get-pipeline-config", err)
		return err
	}

	if !found {
		logger.Debug("pipeline-config-disappeared")
		return nil
	}

	start := time.Now()

	defer func() {
		metric.SchedulingFullDuration{
			PipelineName: s.DB.GetPipelineName(),
			Duration:     time.Since(start),
		}.Emit(logger)
	}()

	versions, err := s.DB.LoadVersionsDB()
	if err != nil {
		logger.Error("failed-to-load-versions-db", err)
		return err
	}

	metric.SchedulingLoadVersionsDuration{
		PipelineName: s.DB.GetPipelineName(),
		Duration:     time.Since(start),
	}.Emit(logger)

	for _, job := range pipelineConfig.Jobs {
		sLog := logger.Session("scheduling", lager.Data{
			"job": job.Name,
		})

		jStart := time.Now()

		defer metric.SchedulingJobDuration{
			PipelineName: s.DB.GetPipelineName(),
			JobName:      job.Name,
			Duration:     time.Since(jStart),
		}.Emit(sLog) // TODO: will this defer do the right thing?

		inputMapping, err := s.InputMapper.SaveNextInputMapping(logger, versions, job)
		if err != nil {
			return err
		}

		for _, inputConfig := range config.JobInputs(job) {
			inputVersion, ok := inputMapping[inputConfig.Name]

			//trigger: true, and the version has not been used
			if ok && inputVersion.FirstOccurrence && inputConfig.Trigger {
				err := s.DB.EnsurePendingBuildExists(job.Name)
				if err != nil {
					logger.Error("failed-to-ensure-pending-build-exists", err)
					return err
				}

				break
			}
		}

		s.BuildStarter.TryStartAllPendingBuilds(logger, job, pipelineConfig.Resources, pipelineConfig.ResourceTypes)
		if err != nil {
			return err
		}
	}

	return nil
}

type Waiter interface {
	Wait()
}

func (s *Scheduler) TriggerImmediately(
	logger lager.Logger,
	jobConfig atc.JobConfig,
	resourceConfigs atc.ResourceConfigs,
	resourceTypes atc.ResourceTypes,
) (db.Build, Waiter, error) {
	logger = logger.Session("trigger-immediately", lager.Data{"job_name": jobConfig.Name})

	lease, leased, err := s.DB.LeaseResourceCheckingForJob(
		logger,
		jobConfig.Name,
		ResourceCheckingForJobTimeout,
	)
	if err != nil {
		logger.Error("failed-to-lease-resource-checking-job", err)
		return db.Build{}, nil, err
	}

	build, err := s.DB.CreateJobBuild(jobConfig.Name)
	if err != nil {
		logger.Error("failed-to-create-job-build", err)
		if leased {
			lease.Break()
		}
		return db.Build{}, nil, err
	}

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		defer wg.Done()

		if leased {
			defer lease.Break()

			jobBuildInputs := config.JobInputs(jobConfig)
			for _, input := range jobBuildInputs {
				scanLog := logger.Session("scan", lager.Data{
					"input":    input.Name,
					"resource": input.Resource,
				})

				err := s.Scanner.Scan(scanLog, input.Resource)
				if err != nil {
					return
				}
			}

			versions, err := s.DB.LoadVersionsDB()
			if err != nil {
				logger.Error("failed-to-load-versions-db", err)
				return
			}

			_, err = s.InputMapper.SaveNextInputMapping(logger, versions, jobConfig)
			if err != nil {
				return
			}

			lease.Break()
		}

		err = s.BuildStarter.TryStartAllPendingBuilds(logger, jobConfig, resourceConfigs, resourceTypes)
	}()

	return build, wg, nil
}

func (s *Scheduler) SaveNextInputMapping(logger lager.Logger, job atc.JobConfig) error {
	versions, err := s.DB.LoadVersionsDB()
	if err != nil {
		logger.Error("failed-to-load-versions-db", err)
		return err
	}

	_, err = s.InputMapper.SaveNextInputMapping(logger, versions, job)
	return err
}
