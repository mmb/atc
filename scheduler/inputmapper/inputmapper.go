package inputmapper

import (
	"github.com/concourse/atc"
	"github.com/concourse/atc/config"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/algorithm"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter . InputMapper

type InputMapper interface {
	SaveNextInputMapping(
		logger lager.Logger,
		versions *algorithm.VersionsDB,
		job atc.JobConfig,
	) (algorithm.InputMapping, error)
}

//go:generate counterfeiter . InputMapperDB

type InputMapperDB interface {
	GetVersionedResourceByVersion(atcVersion atc.Version, resourceName string) (db.SavedVersionedResource, bool, error)
	SaveIndependentInputMapping(inputVersions algorithm.InputMapping, jobName string) error
	SaveNextInputMapping(inputVersions algorithm.InputMapping, jobName string) error
	DeleteNextInputMapping(jobName string) error
}

func NewInputMapper(db InputMapperDB) InputMapper {
	return &inputMapper{db: db}
}

type inputMapper struct {
	db InputMapperDB
}

func (i *inputMapper) SaveNextInputMapping(
	logger lager.Logger,
	versions *algorithm.VersionsDB,
	job atc.JobConfig,
) (algorithm.InputMapping, error) {
	logger = logger.Session("save-next-input-mapping")

	inputConfigs := config.JobInputs(job)

	algorithmInputConfigs, err := i.getAlgorithmInputConfigs(versions, job.Name, inputConfigs)
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

	err = i.db.SaveIndependentInputMapping(independentMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-independent-input-mapping", err)
		return nil, err
	}

	if len(independentMapping) < len(inputConfigs) {
		// this is necessary to prevent builds from running with missing pinned versions
		err := i.db.DeleteNextInputMapping(job.Name)
		if err != nil {
			logger.Error("failed-to-delete-next-input-mapping", err)
		}

		return nil, err
	}

	resolvedMapping, ok := algorithmInputConfigs.Resolve(versions)
	if !ok {
		err := i.db.DeleteNextInputMapping(job.Name)
		if err != nil {
			logger.Error("failed-to-delete-next-input-mapping", err)
		}

		return nil, err
	}

	err = i.db.SaveNextInputMapping(resolvedMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-next-input-mapping", err)
		return nil, err
	}

	return resolvedMapping, nil
}

func (i *inputMapper) getAlgorithmInputConfigs(db *algorithm.VersionsDB, jobName string, inputs []config.JobInput) (algorithm.InputConfigs, error) {
	inputConfigs := algorithm.InputConfigs{}

	for _, input := range inputs {
		if input.Version == nil {
			input.Version = &atc.VersionConfig{Latest: true}
		}

		pinnedVersionID := 0
		if input.Version.Pinned != nil {
			savedVersion, found, err := i.db.GetVersionedResourceByVersion(input.Version.Pinned, input.Resource)
			if err != nil {
				return nil, err
			}

			if !found {
				continue
			}

			pinnedVersionID = savedVersion.ID
		}

		jobs := algorithm.JobSet{}
		for _, passedJobName := range input.Passed {
			jobs[db.JobIDs[passedJobName]] = struct{}{}
		}

		inputConfigs = append(inputConfigs, algorithm.InputConfig{
			Name:            input.Name,
			UseEveryVersion: input.Version.Every,
			PinnedVersionID: pinnedVersionID,
			ResourceID:      db.ResourceIDs[input.Resource],
			Passed:          jobs,
			JobID:           db.JobIDs[jobName],
		})
	}

	return inputConfigs, nil
}
