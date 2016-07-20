package algorithm

import (
	"fmt"
	"time"
)

type VersionsDB struct {
	ResourceVersions []ResourceVersion
	BuildOutputs     []BuildOutput
	BuildInputs      []BuildInput
	JobIDs           map[string]int
	ResourceIDs      map[string]int
	CachedAt         time.Time
}

type ResourceVersion struct {
	VersionID  int
	ResourceID int
	CheckOrder int
}

type BuildOutput struct {
	ResourceVersion
	BuildID int
	JobID   int
}

type BuildInput struct {
	ResourceVersion
	BuildID   int
	JobID     int
	InputName string
}

func (db VersionsDB) IsVersionFirstOccurrence(versionID int, jobID int, inputName string) bool {
	if len(db.BuildInputs) == 0 {
		return true
	}

	for _, buildInput := range db.BuildInputs {
		fmt.Printf("[mylog]: is version first occurrence. buildInput: {versionID: %d, jobID: %d, inputName: %s}, versionID: %d, jobID: %d, inputName: %s",
			buildInput.VersionID, buildInput.JobID, buildInput.InputName,
			versionID, jobID, inputName,
		)
		if buildInput.VersionID != versionID &&
			buildInput.JobID == jobID &&
			buildInput.InputName == inputName {
			fmt.Println("[mylog] is first occurrence")
			return true
		}
	}
	fmt.Println("[mylog] not first occurrence")
	return false
}

func (db VersionsDB) AllVersionsForResource(resourceID int) VersionCandidates {
	candidates := VersionCandidates{}
	for _, output := range db.ResourceVersions {
		if output.ResourceID == resourceID {
			candidates[VersionCandidate{
				VersionID:  output.VersionID,
				CheckOrder: output.CheckOrder,
			}] = struct{}{}
		}
	}

	return candidates
}

func (db VersionsDB) VersionsOfResourcePassedJobs(resourceID int, passed JobSet) VersionCandidates {
	candidates := VersionCandidates{}

	firstTick := true
	for jobID, _ := range passed {
		versions := VersionCandidates{}

		for _, output := range db.BuildOutputs {
			if output.ResourceID == resourceID && output.JobID == jobID {
				versions[VersionCandidate{
					VersionID:  output.VersionID,
					BuildID:    output.BuildID,
					JobID:      output.JobID,
					CheckOrder: output.CheckOrder,
				}] = struct{}{}
			}
		}

		if firstTick {
			candidates = versions
			firstTick = false
		} else {
			candidates = candidates.IntersectByVersion(versions)
		}
	}

	return candidates
}
