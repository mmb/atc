package db

import (
	"github.com/concourse/atc/builds"
	"github.com/concourse/atc/config"
)

type DB interface {
	RegisterJob(name string) error
	RegisterResource(name string) error

	Builds(job string) ([]builds.Build, error)
	GetBuild(job string, build string) (builds.Build, error)
	GetCurrentBuild(job string) (builds.Build, error)

	GetBuildResources(job string, build string) ([]BuildInput, []BuildOutput, error)

	CreateBuild(job string) (builds.Build, error)
	ScheduleBuild(job string, build string, serial bool) (bool, error)
	StartBuild(job string, build string, abortURL string) (bool, error)

	BuildLog(buildID int) ([]byte, error)
	AppendBuildLog(buildID int, log []byte) error

	SaveBuildInput(buildID int, vr builds.VersionedResource) error
	SaveBuildOutput(buildID int, vr builds.VersionedResource) error

	AbortBuild(buildID int) error
	SaveBuildStatus(buildID int, status builds.Status) error

	SaveVersionedResource(builds.VersionedResource) error
	GetLatestVersionedResource(build string) (builds.VersionedResource, error)

	GetLatestInputVersions([]config.Input) (builds.VersionedResources, error)
	GetBuildForInputs(job string, inputs builds.VersionedResources) (builds.Build, error)
	CreateBuildWithInputs(job string, inputs builds.VersionedResources) (builds.Build, error)

	GetNextPendingBuild(job string) (builds.Build, builds.VersionedResources, error)

	GetResourceHistory(resource string) ([]*VersionHistory, error)
}

type BuildInput struct {
	builds.VersionedResource

	FirstOccurrence bool
}

type BuildOutput struct {
	builds.VersionedResource
}

type VersionHistory struct {
	VersionedResource builds.VersionedResource
	Jobs              []*JobHistory
}

type JobHistory struct {
	JobName string
	Builds  []builds.Build
}
