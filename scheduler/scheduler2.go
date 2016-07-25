package scheduler

import (
	"fmt"
	"sync"
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
	CreateJobBuild(jobName string, requireResourceChecking bool) (db.Build, error)
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
	LeaseScheduling(lager.Logger, time.Duration) (db.Lease, bool, error)
	GetPipelineName() string
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

		compromiseInputMapping, complete, err := s.saveNextIndividualAndCompromiseBuildInputs(logger, versions, job)
		if err != nil {
			return err
		}

		if complete {
			_, found, err := s.DB.GetNextPendingBuild(job.Name)
			if err != nil {
				logger.Error("failed-to-get-next-pending-build", err)
				return err
			}

			if !found {
				inputConfigs := config.JobInputs(job)
				trigger := false
				for _, inputConfig := range inputConfigs {
					//trigger: true, and the version has not been used
					if inputConfig.Trigger && compromiseInputMapping[inputConfig.Name].FirstOccurrence {
						trigger = true
						break
					}
				}

				if trigger {
					_, err := s.DB.CreateJobBuild(job.Name, false)
					if err != nil {
						logger.Error("failed-to-create-job-build", err)
						return err
					}
				}
			}
		}

		for true {
			started, err := s.tryStartNextPendingBuild(logger, job, pipelineConfig.Resources, pipelineConfig.ResourceTypes, false)
			if err != nil {
				return err
			}
			if !started {
				logger.Info("finished-try-starting-pending-builds")
				break
			}
		}

		metric.SchedulingJobDuration{
			PipelineName: s.DB.GetPipelineName(),
			JobName:      job.Name,
			Duration:     time.Since(jStart),
		}.Emit(sLog)
	}

	return nil
}

func (s *Scheduler) tryStartNextPendingBuild(
	logger lager.Logger,
	jobConfig atc.JobConfig,
	resourceConfigs atc.ResourceConfigs,
	resourceTypes atc.ResourceTypes,
	immediate bool,
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

	acquired, err := s.DB.LeaseResourceCheckingForJobAcquired(jobConfig.Name)
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

	buildInputs, err := s.DB.GetCompromiseBuildInputs(jobConfig.Name)
	if err != nil {
		logger.Error("failed-to-get-compromise-build-inputs", err)
		return false, nil
	}

	err = s.DB.UseInputsForBuild(nextPendingBuild.ID, buildInputs)
	if err != nil {
		return false, nil
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

	build, err := s.DB.CreateJobBuild(jobConfig.Name, true) // require resource checking
	if err != nil {
		logger.Error("failed-to-create-job-build", err)
		return db.Build{}, err
	}

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()

		lease, created, err := s.DB.LeaseResourceCheckingForJob(logger, jobConfig.Name, 5*time.Minute)
		if err != nil {
			logger.Error("failed-to-lease-resource-checking-job", err)
			return
		}

		if !created {
			return
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
				lease.Break()
				return
			}
		}

		versions, err := s.DB.LoadVersionsDB()
		if err != nil {
			logger.Error("failed-to-load-versions-db", err)
			lease.Break()
			return
		}

		_, complete, err := s.saveNextIndividualAndCompromiseBuildInputs(logger, versions, jobConfig)
		if err != nil {
			lease.Break()
			return
		}

		if !complete {
			logger.Info("not-enough-compromise-inputs")
			lease.Break()
			return
		}

		lease.Break()

		for true {
			started, err := s.tryStartNextPendingBuild(logger, jobConfig, resourceConfigs, resourceTypes, true)
			if err != nil {
				return
			}
			if !started {
				logger.Info("finished-try-starting-pending-builds")
				break
			}
		}
	}()

	return build, nil
}

func (s *Scheduler) saveNextIndividualAndCompromiseBuildInputs(
	logger lager.Logger,
	versions *algorithm.VersionsDB,
	job atc.JobConfig,
) (algorithm.InputMapping, bool, error) {
	logger = logger.Session("save-next-ideal-and-compromise-build-inputs")

	inputConfigs := config.JobInputs(job)

	algorithmInputConfigs, err := s.DB.GetAlgorithmInputConfigs(versions, job.Name, inputConfigs)
	logger.Debug(fmt.Sprintf("[mylog] algo input configs: %#v", algorithmInputConfigs))

	if err != nil {
		logger.Error("failed-to-get-algorithm-input-configs", err)
		return nil, false, err
	}

	idealMapping := algorithm.InputMapping{}
	for _, inputConfig := range algorithmInputConfigs {
		singletonMapping, ok := algorithm.InputConfigs{inputConfig}.Resolve(versions)
		if ok {
			idealMapping[inputConfig.Name] = singletonMapping[inputConfig.Name]
		}
	}

	logger.Debug(fmt.Sprintf("[mylog] ideal input versions: %#v", idealMapping))
	err = s.DB.SaveIdealInputVersions(idealMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-ideal-input-versions", err)
		return nil, false, err
	}

	//if there is not an ideal version for every input
	if len(idealMapping) < len(inputConfigs) { // important to not compare it to len(algorithmInputConfigs)
		return nil, false, nil
	}

	compromiseMapping, ok := algorithmInputConfigs.Resolve(versions)
	logger.Debug(fmt.Sprintf("[mylog] compromise input versions: %#v", compromiseMapping))
	if !ok {
		err := s.DB.DeleteCompromiseInputVersions(job.Name)
		if err != nil {
			logger.Error("failed-to-delete-compromise-input-versions", err)
			return nil, false, err
		}

		return nil, false, nil
	}

	err = s.DB.SaveCompromiseInputVersions(compromiseMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-compromise-input-versions", err)
		return nil, false, err
	}

	return compromiseMapping, true, nil
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
