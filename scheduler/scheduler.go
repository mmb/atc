package scheduler

import (
	"time"

	"github.com/concourse/atc"
	"github.com/concourse/atc/config"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/algorithm"
	"github.com/concourse/atc/engine"
	"github.com/concourse/atc/metric"
	"github.com/pivotal-golang/lager"
)

type Scheduler struct {
	DB       SchedulerDB
	BuildsDB SchedulerBuildsDB
	Factory  BuildFactory
	Engine   engine.Engine
	Scanner  Scanner
}

//go:generate counterfeiter . SchedulerDB

type SchedulerDB interface {
	LeaseScheduling(lager.Logger, time.Duration) (db.Lease, bool, error)
	LoadVersionsDB() (*algorithm.VersionsDB, error)
	GetPipelineName() string
	GetConfig() (atc.Config, db.ConfigVersion, bool, error)
	GetAlgorithmInputConfigs(db *algorithm.VersionsDB, jobName string, inputs []config.JobInput) (algorithm.InputConfigs, error)
	EnsurePendingBuildExists(jobName string) error
	CreateJobBuild(jobName string) (db.Build, error)
	GetNextPendingBuild(job string) (db.Build, bool, error)
	SaveIndependentInputMapping(inputVersions algorithm.InputMapping, jobName string) error
	SaveNextInputMapping(inputVersions algorithm.InputMapping, jobName string) error
	DeleteNextInputMapping(jobName string) error
	GetNextBuildInputs(jobName string) ([]db.BuildInput, bool, error)
	LeaseResourceCheckingForJob(logger lager.Logger, job string, interval time.Duration) (db.Lease, bool, error)
	IsPaused() (bool, error)
	GetJob(job string) (db.SavedJob, error)
	GetRunningBuildsBySerialGroup(jobName string, serialGroups []string) ([]db.Build, error)
	GetNextPendingBuildBySerialGroup(jobName string, serialGroups []string) (db.Build, bool, error)
	UpdateBuildToScheduled(int) (bool, error)
	UseInputsForBuild(buildID int, inputs []db.BuildInput) error // TODO: replace it with a job-wide column
}

//go:generate counterfeiter . SchedulerBuildsDB

type SchedulerBuildsDB interface {
	FinishBuild(buildID int, pipelineID int, status db.Status) error
}

//go:generate counterfeiter . Scanner

type Scanner interface {
	Scan(lager.Logger, string) error
}

//go:generate counterfeiter . BuildFactory

type BuildFactory interface {
	Create(atc.JobConfig, atc.ResourceConfigs, atc.ResourceTypes, []db.BuildInput) (atc.Plan, error)
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
		}.Emit(sLog)

		inputMapping, err := s.saveNextInputMapping(logger, versions, job)
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

		s.tryStartAllPendingBuilds(logger, job, pipelineConfig.Resources, pipelineConfig.ResourceTypes)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Scheduler) tryStartAllPendingBuilds(
	logger lager.Logger,
	jobConfig atc.JobConfig,
	resourceConfigs atc.ResourceConfigs,
	resourceTypes atc.ResourceTypes,
) error {
	started := true
	for started {
		var err error
		started, err = s.tryStartNextPendingBuild(logger, jobConfig, resourceConfigs, resourceTypes)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Scheduler) tryStartNextPendingBuild(
	logger lager.Logger,
	jobConfig atc.JobConfig,
	resourceConfigs atc.ResourceConfigs,
	resourceTypes atc.ResourceTypes,
) (bool, error) {
	logger = logger.Session("try-start-next-pending-build")
	nextPendingBuild, found, err := s.DB.GetNextPendingBuild(jobConfig.Name)
	if err != nil {
		logger.Error("failed-to-get-next-pending-build", err)
		return false, err
	}
	if !found {
		return false, nil
	}

	buildInputs, found, err := s.DB.GetNextBuildInputs(jobConfig.Name)
	if err != nil {
		logger.Error("failed-to-get-next-build-inputs", err)
		return false, err
	}
	if !found {
		return false, nil
	}

	pipelinePaused, err := s.DB.IsPaused()
	if err != nil {
		logger.Error("failed-to-check-if-pipeline-is-paused", err)
		return false, err
	}
	if pipelinePaused {
		return false, nil
	}

	job, err := s.DB.GetJob(jobConfig.Name)
	if err != nil {
		logger.Error("failed-to-check-if-job-is-paused", err)
		return false, err
	}
	if job.Paused {
		return false, nil
	}

	reachedMaxInFlight, err := s.isMaxInFlightReached(logger, jobConfig, nextPendingBuild.ID) //TODO: set it or compute it on the fly
	if err != nil {
		return false, err
	}
	if reachedMaxInFlight {
		return false, nil
	}

	updated, err := s.DB.UpdateBuildToScheduled(nextPendingBuild.ID)
	if err != nil {
		logger.Error("failed-to-update-build-to-scheduled", err, lager.Data{"build-id": nextPendingBuild.ID})
		return false, err
	}

	if !updated {
		logger.Debug("build-already-scheduled", lager.Data{"build-id": nextPendingBuild.ID})
		return false, nil
	}

	err = s.DB.UseInputsForBuild(nextPendingBuild.ID, buildInputs)
	if err != nil {
		return false, err
	}

	plan, err := s.Factory.Create(jobConfig, resourceConfigs, resourceTypes, buildInputs)
	if err != nil {
		// Don't use ErrorBuild because it logs a build event, and this build hasn't started
		err := s.BuildsDB.FinishBuild(nextPendingBuild.ID, nextPendingBuild.PipelineID, db.StatusErrored)
		if err != nil {
			logger.Error("failed-to-mark-build-as-errored", err, lager.Data{"build-id": nextPendingBuild.ID})
		}
		return false, nil
	}

	createdBuild, err := s.Engine.CreateBuild(logger, nextPendingBuild, plan)
	if err != nil {
		logger.Error("failed-to-create-build", err, lager.Data{"build-id": nextPendingBuild.ID})
		return false, nil
	}

	logger.Info("building-build")
	go createdBuild.Resume(logger)

	return true, nil
}

func (s *Scheduler) TriggerImmediately(
	logger lager.Logger,
	jobConfig atc.JobConfig,
	resourceConfigs atc.ResourceConfigs,
	resourceTypes atc.ResourceTypes,
) (db.Build, error) {
	logger = logger.Session("trigger-immediately", lager.Data{"job_name": jobConfig.Name})

	lease, leased, err := s.DB.LeaseResourceCheckingForJob(logger, jobConfig.Name, 5*time.Minute)
	if err != nil {
		logger.Error("failed-to-lease-resource-checking-job", err)
		return db.Build{}, err
	}

	build, err := s.DB.CreateJobBuild(jobConfig.Name)
	if err != nil {
		logger.Error("failed-to-create-job-build", err)
		if leased {
			lease.Break()
		}
		return db.Build{}, err
	}

	go func() {
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

			_, err = s.saveNextInputMapping(logger, versions, jobConfig)
			if err != nil {
				return
			}

			lease.Break()
		}

		err = s.tryStartAllPendingBuilds(logger, jobConfig, resourceConfigs, resourceTypes)
	}()

	return build, nil
}

func (s *Scheduler) SaveNextInputMapping(logger lager.Logger, job atc.JobConfig) error {
	versions, err := s.DB.LoadVersionsDB()
	if err != nil {
		logger.Error("failed-to-load-versions-db", err)
		return err
	}

	_, err = s.saveNextInputMapping(logger, versions, job)
	return err
}

func (s *Scheduler) saveNextInputMapping(
	logger lager.Logger,
	versions *algorithm.VersionsDB,
	job atc.JobConfig,
) (algorithm.InputMapping, error) {
	logger = logger.Session("save-next-input-mapping")

	inputConfigs := config.JobInputs(job)

	algorithmInputConfigs, err := s.DB.GetAlgorithmInputConfigs(versions, job.Name, inputConfigs)
	if err != nil {
		logger.Error("failed-to-get-algorithm-input-configs", err)
		return nil, err
	}

	independentMapping := algorithm.InputMapping{}
	for _, inputConfig := range algorithmInputConfigs {
		singletonMapping, ok := algorithm.InputConfigs{inputConfig}.Resolve(versions)
		if ok {
			independentMapping[inputConfig.Name] = singletonMapping[inputConfig.Name]
		}
	}

	err = s.DB.SaveIndependentInputMapping(independentMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-independent-input-mapping", err)
		return nil, err
	}

	if len(independentMapping) < len(inputConfigs) {
		// this is necessary to prevent builds from running with missing pinned versions
		err := s.DB.DeleteNextInputMapping(job.Name)
		if err != nil {
			logger.Error("failed-to-delete-next-input-mapping", err)
		}

		return nil, err
	}

	resolvedMapping, ok := algorithmInputConfigs.Resolve(versions)
	if !ok {
		err := s.DB.DeleteNextInputMapping(job.Name)
		if err != nil {
			logger.Error("failed-to-delete-next-input-mapping", err)
		}

		return nil, err
	}

	err = s.DB.SaveNextInputMapping(resolvedMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-next-input-mapping", err)
		return nil, err
	}

	return resolvedMapping, nil
}

func (s *Scheduler) isMaxInFlightReached(logger lager.Logger, jobConfig atc.JobConfig, buildID int) (bool, error) {
	logger = logger.Session("is-max-in-flight-reached")
	maxInFlight := jobConfig.MaxInFlight()

	if maxInFlight > 0 {
		builds, err := s.DB.GetRunningBuildsBySerialGroup(jobConfig.Name, jobConfig.GetSerialGroups())
		if err != nil {
			logger.Error("failed-to-get-running-builds-by-serial-group", err)
			return false, err
		}

		if len(builds) >= maxInFlight {
			return true, nil
		}

		nextMostPendingBuild, found, err := s.DB.GetNextPendingBuildBySerialGroup(jobConfig.Name, jobConfig.GetSerialGroups())
		if err != nil {
			logger.Error("failed-to-get-next-pending-build-by-serial-group", err)
			return false, err
		}

		if !found {
			logger.Debug("pending-build-disappeared-from-serial-group") // TODO: refactor serial groups
			return true, nil
		}

		return nextMostPendingBuild.ID != buildID, nil
	}

	return false, nil
}
