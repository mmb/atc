package scheduler

import (
	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/engine"
	"github.com/pivotal-golang/lager"
)

type BuildStarter struct {
	DB       BuildStarterDB
	BuildsDB BuildStarterBuildsDB
	Factory  BuildFactory
	Engine   engine.Engine
}

//go:generate counterfeiter . BuildStarterDB

type BuildStarterDB interface {
	GetNextPendingBuild(job string) (db.Build, bool, error)
	SetMaxInFlightReached(jobName string, reached bool) error
	GetNextBuildInputs(jobName string) ([]db.BuildInput, bool, error)
	IsPaused() (bool, error)
	GetJob(job string) (db.SavedJob, error)
	GetRunningBuildsBySerialGroup(jobName string, serialGroups []string) ([]db.Build, error)
	GetNextPendingBuildBySerialGroup(jobName string, serialGroups []string) (db.Build, bool, error)
	UpdateBuildToScheduled(int) (bool, error)
	UseInputsForBuild(buildID int, inputs []db.BuildInput) error // TODO: replace it with a job-wide column
}

//go:generate counterfeiter . BuildStarterBuildsDB

type BuildStarterBuildsDB interface {
	FinishBuild(buildID int, pipelineID int, status db.Status) error
}

//go:generate counterfeiter . BuildFactory

type BuildFactory interface {
	Create(atc.JobConfig, atc.ResourceConfigs, atc.ResourceTypes, []db.BuildInput) (atc.Plan, error)
}

func (s *BuildStarter) TryStartAllPendingBuilds(
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

func (s *BuildStarter) tryStartNextPendingBuild(
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

	reachedMaxInFlight, err := s.isMaxInFlightReached(logger, jobConfig, nextPendingBuild.ID) //TODO: set it or compute it on the fly
	if err != nil {
		return false, err
	}

	err = s.DB.SetMaxInFlightReached(jobConfig.Name, reachedMaxInFlight)
	if err != nil {
		return false, err
	}

	if reachedMaxInFlight {
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
	/*go */ createdBuild.Resume(logger)

	return true, nil
}

func (s *BuildStarter) isMaxInFlightReached(logger lager.Logger, jobConfig atc.JobConfig, buildID int) (bool, error) {
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
