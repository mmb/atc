package scheduler

import (
	"github.com/concourse/atc"
	"github.com/concourse/atc/config"
	"github.com/concourse/atc/db/algorithm"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter . InputMapperDB

type InputMapperDB interface {
	GetAlgorithmInputConfigs(db *algorithm.VersionsDB, jobName string, inputs []config.JobInput) (algorithm.InputConfigs, error)
	SaveIndependentInputMapping(inputVersions algorithm.InputMapping, jobName string) error
	SaveNextInputMapping(inputVersions algorithm.InputMapping, jobName string) error
	DeleteNextInputMapping(jobName string) error
}

type InputMapper struct {
	DB InputMapperDB
}

func (i *InputMapper) SaveNextInputMapping(
	logger lager.Logger,
	versions *algorithm.VersionsDB,
	job atc.JobConfig,
) (algorithm.InputMapping, error) {
	logger = logger.Session("save-next-input-mapping")

	inputConfigs := config.JobInputs(job)

	algorithmInputConfigs, err := i.DB.GetAlgorithmInputConfigs(versions, job.Name, inputConfigs)
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

	err = i.DB.SaveIndependentInputMapping(independentMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-independent-input-mapping", err)
		return nil, err
	}

	if len(independentMapping) < len(inputConfigs) {
		// this is necessary to prevent builds from running with missing pinned versions
		err := i.DB.DeleteNextInputMapping(job.Name)
		if err != nil {
			logger.Error("failed-to-delete-next-input-mapping", err)
		}

		return nil, err
	}

	resolvedMapping, ok := algorithmInputConfigs.Resolve(versions)
	if !ok {
		err := i.DB.DeleteNextInputMapping(job.Name)
		if err != nil {
			logger.Error("failed-to-delete-next-input-mapping", err)
		}

		return nil, err
	}

	err = i.DB.SaveNextInputMapping(resolvedMapping, job.Name)
	if err != nil {
		logger.Error("failed-to-save-next-input-mapping", err)
		return nil, err
	}

	return resolvedMapping, nil
}
