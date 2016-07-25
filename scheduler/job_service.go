package scheduler

//
// import (
// 	"fmt"
// 	"time"
//
// 	"github.com/concourse/atc"
// 	"github.com/concourse/atc/config"
// 	"github.com/concourse/atc/db"
// 	"github.com/concourse/atc/db/algorithm"
// 	"github.com/pivotal-golang/lager"
// )
//
// //go:generate counterfeiter . JobServiceDB
//
// type JobServiceDB interface {
// 	GetJob(job string) (db.SavedJob, error)
// 	GetRunningBuildsBySerialGroup(jobName string, serialGroups []string) ([]db.Build, error)
// 	GetNextPendingBuildBySerialGroup(jobName string, serialGroups []string) (db.Build, bool, error)
// 	UpdateBuildPreparation(prep db.BuildPreparation) error
// 	IsPaused() (bool, error)
//
// 	LoadVersionsDB() (*algorithm.VersionsDB, error)
// 	GetNextInputVersions(versions *algorithm.VersionsDB, job string, inputs []config.JobInput) ([]db.BuildInput, bool, db.MissingInputReasons, error)
// 	GetMissingInputReasons(
// 		algorithmInputConfigs algorithm.InputConfigs,
// 		inputs []config.JobInput,
// 		idealMapping algorithm.InputMapping,
// 	) (db.MissingInputReasons, error)
// 	GetBuildPreparation(int) (db.BuildPreparation, bool, error)
// 	GetIdealBuildInputs(jobName string) ([]db.BuildInput, error)
// 	GetCompromiseBuildInputs(jobName string) ([]db.BuildInput, error)
// 	UseInputsForBuild(buildID int, inputs []db.BuildInput) error
// }
//
// //go:generate counterfeiter . JobService
//
// type JobService interface {
// 	CanBuildBeScheduled(
// 		logger lager.Logger,
// 		build db.Build,
// 		buildPrep db.BuildPreparation,
// 		versions *algorithm.VersionsDB,
// 		buildInputs []db.BuildInput,
// 	) (bool, string, error)
// }
//
// type jobService struct {
// 	JobConfig atc.JobConfig
// 	DBJob     db.SavedJob
// 	DB        JobServiceDB
// 	Scanner   Scanner
// }
//
// func NewJobService(config atc.JobConfig, jobServiceDB JobServiceDB, scanner Scanner) (JobService, error) {
// 	job, err := jobServiceDB.GetJob(config.Name)
// 	if err != nil {
// 		return jobService{}, err
// 	}
//
// 	return jobService{
// 		JobConfig: config,
// 		DBJob:     job,
// 		DB:        jobServiceDB,
// 		Scanner:   scanner,
// 	}, nil
// }
//
// func (s jobService) updateBuildPrepAndReturn(buildPrep db.BuildPreparation, scheduled bool, message string) (bool, string, error) {
// 	err := s.DB.UpdateBuildPreparation(buildPrep)
// 	if err != nil {
// 		return false, fmt.Sprintf("update-build-prep-db-failed-when-%s", message), err
// 	}
// 	return scheduled, message, nil
// }
//
// func (s jobService) maybeScanResource(
// 	logger lager.Logger,
// 	build db.Build,
// 	buildPrep db.BuildPreparation,
// 	versions *algorithm.VersionsDB,
// 	buildInputs []db.BuildInput,
// ) (db.BuildPreparation, string, error) {
// 	jobBuildInputs := config.JobInputs(s.JobConfig)
// 	if versions == nil {
// 		for _, input := range jobBuildInputs {
// 			buildPrep.Inputs[input.Name] = db.BuildPreparationStatusUnknown
// 		}
//
// 		err := s.DB.UpdateBuildPreparation(buildPrep)
// 		if err != nil {
// 			return buildPrep, "failed-to-update-build-prep-with-inputs", err
// 		}
//
// 		for _, input := range jobBuildInputs {
// 			scanLog := logger.Session("scan", lager.Data{
// 				"input":    input.Name,
// 				"resource": input.Resource,
// 			})
//
// 			buildPrep = s.cloneBuildPrep(buildPrep)
// 			buildPrep.Inputs[input.Name] = db.BuildPreparationStatusBlocking
// 			err := s.DB.UpdateBuildPreparation(buildPrep)
// 			if err != nil {
// 				return buildPrep, "failed-to-update-build-prep-with-blocking-input", err
// 			}
//
// 			logger.Debug("[mylog] scanning", lager.Data{"resource": input.Resource})
// 			err = s.Scanner.Scan(scanLog, input.Resource)
// 			if err != nil {
// 				return buildPrep, "failed-to-scan", err
// 			}
//
// 			buildPrep = s.cloneBuildPrep(buildPrep)
// 			buildPrep.Inputs[input.Name] = db.BuildPreparationStatusNotBlocking
// 			err = s.DB.UpdateBuildPreparation(buildPrep)
// 			if err != nil {
// 				return buildPrep, "failed-to-update-build-prep-with-not-blocking-input", err
// 			}
//
// 			scanLog.Info("done")
// 		}
//
// 		loadStart := time.Now()
//
// 		vLog := logger.Session("loading-versions")
// 		vLog.Info("start")
//
// 		versions, err = s.DB.LoadVersionsDB()
// 		if err != nil {
// 			vLog.Error("failed", err)
// 			return buildPrep, "failed-to-load-versions-db", err
// 		}
//
// 		vLog.Info("done", lager.Data{"took": time.Since(loadStart).String()})
// 	} else {
// 		for _, input := range jobBuildInputs {
// 			buildPrep.Inputs[input.Name] = db.BuildPreparationStatusNotBlocking
// 		}
// 		err := s.DB.UpdateBuildPreparation(buildPrep)
// 		if err != nil {
// 			return buildPrep, "failed-to-update-build-prep-with-discovered-inputs", err
// 		}
// 	}
//
// 	buildPrep.InputsSatisfied = db.BuildPreparationStatusBlocking
// 	err := s.DB.UpdateBuildPreparation(buildPrep)
// 	if err != nil {
// 		return buildPrep, "failed-to-update-build-prep-with-discovered-inputs", err
// 	}
//
// 	err = s.DB.UseInputsForBuild(build.ID, buildInputs)
// 	if err != nil {
// 		return buildPrep, "failed-to-use-inputs-for-build", err
// 	}
//
// 	buildPrep.InputsSatisfied = db.BuildPreparationStatusNotBlocking
// 	err = s.DB.UpdateBuildPreparation(buildPrep)
// 	if err != nil {
// 		return buildPrep, "failed-to-update-build-prep-with-inputs-satisfied", err
// 	}
//
// 	return buildPrep, "", nil
// }
//
// //pipeline not paused
// //job not paused
// //build status pending
// //max in flight not exceeded
// func (s jobService) CanBuildBeScheduled(
// 	logger lager.Logger,
// 	build db.Build,
// 	buildPrep db.BuildPreparation,
// 	versions *algorithm.VersionsDB,
// 	buildInputs []db.BuildInput,
// ) (bool, string, error) {
// 	if build.Status != db.StatusPending {
// 		return false, "build-not-pending", nil
// 	}
//
// 	buildPrep, message, err := s.maybeScanResource(logger, build, buildPrep, versions, buildInputs)
// 	if err != nil || message != "" {
// 		return false, message, err
// 	}
//
// 	buildPrep.MaxRunningBuilds = db.BuildPreparationStatusBlocking
// 	err = s.DB.UpdateBuildPreparation(buildPrep)
// 	if err != nil {
// 		return false, "update-build-prep-db-failed-pipeline-not-paused", err
// 	}
//
// 	maxInFlight := s.JobConfig.MaxInFlight()
//
// 	if maxInFlight > 0 {
// 		builds, err := s.DB.GetRunningBuildsBySerialGroup(s.DBJob.Name, s.JobConfig.GetSerialGroups())
// 		if err != nil {
// 			return false, "db-failed", err
// 		}
//
// 		if len(builds) >= maxInFlight {
// 			buildPrep.MaxRunningBuilds = db.BuildPreparationStatusBlocking
// 			return s.updateBuildPrepAndReturn(buildPrep, false, "max-in-flight-reached")
// 		}
//
// 		nextMostPendingBuild, found, err := s.DB.GetNextPendingBuildBySerialGroup(s.DBJob.Name, s.JobConfig.GetSerialGroups())
// 		if err != nil {
// 			return false, "db-failed", err
// 		}
//
// 		if !found {
// 			return false, "no-pending-build", nil
// 		}
//
// 		if nextMostPendingBuild.ID != build.ID {
// 			return false, "not-next-most-pending", nil
// 		}
// 	}
//
// 	buildPrep.MaxRunningBuilds = db.BuildPreparationStatusNotBlocking
// 	err = s.DB.UpdateBuildPreparation(buildPrep)
// 	if err != nil {
// 		return false, "update-build-prep-db-failed-not-max-running-builds", err
// 	}
//
// 	return true, "can-be-scheduled", nil
// }
//
// // Turns out that counterfieter clones the pointer in the build prep so when
// // the build prep gets modified, so does the copy in the fake. This clone is
// // done to get around this. God damn it counterfeiter.
// func (s *jobService) cloneBuildPrep(buildPrep db.BuildPreparation) db.BuildPreparation {
// 	clone := db.BuildPreparation{
// 		BuildID:          buildPrep.BuildID,
// 		PausedPipeline:   buildPrep.PausedPipeline,
// 		PausedJob:        buildPrep.PausedJob,
// 		MaxRunningBuilds: buildPrep.MaxRunningBuilds,
// 		Inputs:           map[string]db.BuildPreparationStatus{},
// 		InputsSatisfied:  buildPrep.InputsSatisfied,
// 	}
//
// 	for key, value := range buildPrep.Inputs {
// 		clone.Inputs[key] = value
// 	}
//
// 	return clone
// }
