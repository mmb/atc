package scheduler

import (
	"time"

	"github.com/concourse/atc"
	"github.com/concourse/atc/config"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/algorithm"
	"github.com/concourse/atc/engine"
	"github.com/pivotal-golang/lager"
)

type Scheduler2 struct {
	DB       Scheduler2DB
	BuildsDB BuildsDB2
	Factory  BuildFactory
	Engine   engine.Engine
	Scanner  Scanner2
}

type Scheduler2DB interface {
	CreateJobBuild(jobName string) (db.Build, error)
	GetNextPendingBuild(job string) (db.Build, bool, error)
	IsPaused() (bool, error)
	GetConfig() (atc.Config, db.ConfigVersion, bool, error)
	GetJob(job string) (db.SavedJob, error)
	GetRunningBuildsBySerialGroup(jobName string, serialGroups []string) ([]db.Build, error)
	GetNextPendingBuildBySerialGroup(jobName string, serialGroups []string) (db.Build, bool, error)
	GetCompromiseBuildInputs(jobName string) ([]db.BuildInput, error)
	UpdateBuildToScheduled(int) (bool, error)
	UseInputsForBuild(buildID int, inputs []db.BuildInput) error // TODO: replace it with a job-wide column
	LeaseResourceCheckingForJob(logger lager.Logger, job string, interval time.Duration) (db.Lease, bool, error)
	LeaseResourceCheckingForJobAcquired(jobName string) (bool, error)
	GetAlgorithmInputConfigs(db *algorithm.VersionsDB, jobName string, inputs []config.JobInput) (algorithm.InputConfigs, error)
	SaveIdealInputVersions(inputVersions algorithm.InputMapping, jobName string) error
	SaveCompromiseInputVersions(inputVersions algorithm.InputMapping, jobName string) error
	DeleteCompromiseInputVersions(jobName string) error
	LoadVersionsDB() (*algorithm.VersionsDB, error)
}

type BuildsDB2 interface {
	FinishBuild(buildID int, pipelineID int, status db.Status) error
}

type Scanner2 interface {
	Scan(lager.Logger, string) error
}

func (s *Scheduler2) isMaxInFlightReached(logger lager.Logger, jobConfig atc.JobConfig, buildID int) (bool, error) {
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
			logger.Debug("pending-build-disappeared-from-serial-group") // TODO: existing bug?
			return true, nil
		}

		return nextMostPendingBuild.ID != buildID, nil
	}

	return false, nil
}

// !pipeline paused
// !job paused
// get next pending build

func (s *Scheduler2) TryStartNextPendingBuild(logger lager.Logger, jobName string) (bool, error) {
	logger = logger.Session("try-start-next-pending-build")
	nextPendingBuild, found, err := s.DB.GetNextPendingBuild(jobName)
	if err != nil {
		logger.Error("failed-to-get-next-pending-build", err)
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

	job, err := s.DB.GetJob(jobName)
	if err != nil {
		logger.Error("failed-to-check-if-job-is-paused", err)
		return false, err
	}
	if job.Paused {
		return false, nil
	}

	pipelineConfig, _, found, err := s.DB.GetConfig()
	if err != nil {
		logger.Error("failed-to-get-pipeline-config", err)
		return false, err
	}
	if !found {
		logger.Info("pipeline-config-disappeared")
		return false, nil
	}

	jobConfig, found := pipelineConfig.Jobs.Lookup(jobName)
	if !found {
		logger.Info("job-config-disappeared")
		return false, nil
	}

	reachedMaxInFlight, err := s.isMaxInFlightReached(logger, jobConfig, nextPendingBuild.ID) //TODO: set it or compute it on the fly
	if err != nil {
		return false, err
	}
	if reachedMaxInFlight {
		return false, nil
	}

	acquired, err := s.DB.LeaseResourceCheckingForJobAcquired(jobName)
	if err != nil {
		logger.Error("failed-to-check-if-resource-checking-lease-for-job-is-acquired", err)
		return false, err
	}
	if acquired {
		logger.Debug("resource-checking-for-job-already-acquired")
		return false, nil
	}

	updated, err := s.DB.UpdateBuildToScheduled(nextPendingBuild.ID)
	if err != nil {
		logger.Error("failed-to-update-build-to-scheduled", err, lager.Data{"build-id": nextPendingBuild.ID})
		return false, nil
	}

	if !updated {
		logger.Debug("build-already-scheduled", lager.Data{"build-id": nextPendingBuild.ID})
		return false, nil
	}

	buildInputs, err := s.DB.GetCompromiseBuildInputs(jobName)
	if err != nil {
		logger.Error("failed-to-get-compromise-build-inputs", err)
		return false, nil
	}

	err = s.DB.UseInputsForBuild(nextPendingBuild.ID, buildInputs)
	if err != nil {
		return false, nil
	}

	plan, err := s.Factory.Create(jobConfig, pipelineConfig.Resources, pipelineConfig.ResourceTypes, buildInputs)
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

func (s *Scheduler2) TriggerImmediately(logger lager.Logger, jobConfig atc.JobConfig) error {
	logger = logger.Session("trigger-immediately", lager.Data{"job_name": jobConfig.Name})
	lease, created, err := s.DB.LeaseResourceCheckingForJob(logger, jobConfig.Name, 5*time.Minute)
	if err != nil {
		logger.Error("failed-to-lease-resource-checking-job", err)
		return err
	}

	if !created {
		logger.Debug("failed-to-resource-checking-lease-for-job")
		return nil
	}

	_, err = s.DB.CreateJobBuild(jobConfig.Name)
	if err != nil {
		logger.Error("failed-to-create-job-build", err)
		return err
	}

	jobBuildInputs := config.JobInputs(jobConfig)
	for _, input := range jobBuildInputs {
		scanLog := logger.Session("scan", lager.Data{
			"input":    input.Name,
			"resource": input.Resource,
		})

		err := s.Scanner.Scan(scanLog, input.Resource)
		if err != nil {
			logger.Error("failed-to-scan-resource", err, lager.Data{"resource": input.Resource})
			return err
		}
	}

	lease.Break()
	versions, err := s.DB.LoadVersionsDB()
	if err != nil {
		logger.Error("failed-to-load-versions-db", err)
		return err
	}

	complete, err := s.saveNextIndividualAndCompromiseBuildInputs(logger, versions, jobConfig)
	if err != nil {
		return err
	}

	if !complete {
		logger.Info("not-enough-compromise-inputs")
		return nil
	}

	for true {
		started, err := s.TryStartNextPendingBuild(logger, jobConfig.Name)
		if err != nil {
			return err
		}
		if !started {
			logger.Info("finished-try-starting-pending-builds")
			break
		}
	}

	return nil
}

func (s *Scheduler2) saveNextIndividualAndCompromiseBuildInputs(
	logger lager.Logger,
	versions *algorithm.VersionsDB,
	job atc.JobConfig,
) (bool, error) {
	logger = logger.Session("save-next-ideal-and-compromise-build-inputs")

	inputConfigs := config.JobInputs(job)

	algorithmInputConfigs, err := s.DB.GetAlgorithmInputConfigs(versions, job.Name, inputConfigs)
	if err != nil {
		logger.Error("failed-to-get-algorithm-input-configs", err)
		return false, err
	}

	idealMapping := algorithm.InputMapping{}
	for _, inputConfig := range algorithmInputConfigs {
		singletonMapping, ok := algorithm.InputConfigs{inputConfig}.Resolve(versions)
		if ok {
			idealMapping[inputConfig.Name] = singletonMapping[inputConfig.Name]
		}
	}

	err = s.DB.SaveIdealInputVersions(idealMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-ideal-input-versions", err)
		return false, err
	}

	//if there is not an ideal version for every input
	if len(idealMapping) < len(inputConfigs) { // important to not compare it to len(algorithmInputConfigs)
		return false, nil
	}

	compromiseMapping, ok := algorithmInputConfigs.Resolve(versions)
	if !ok {
		err := s.DB.DeleteCompromiseInputVersions(job.Name)
		if err != nil {
			logger.Error("failed-to-delete-compromise-input-versions", err)
			return false, err
		}

		return false, nil
	}

	err = s.DB.SaveCompromiseInputVersions(compromiseMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-compromise-input-versions", err)
		return false, err
	}

	return true, nil
}
