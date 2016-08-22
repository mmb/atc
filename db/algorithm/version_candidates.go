package algorithm

import (
	"fmt"
	"log"
	"sort"
)

type VersionCandidate struct {
	VersionID  int
	BuildID    int
	JobID      int
	CheckOrder int
}

func (candidate VersionCandidate) String() string {
	return fmt.Sprintf("{v%d, j%db%d}", candidate.VersionID, candidate.JobID, candidate.BuildID)
}

type VersionCandidates struct {
	versions    versions
	constraints constraints
	buildIDs    map[int]BuildSet
}

type versions []version

type version struct {
	id     int
	order  int
	passed map[int]BuildSet
}

func (v version) passedAny(jobID int, builds BuildSet) bool {
	bs, found := v.passed[jobID]
	if !found {
		return true
	}

	return bs.Overlaps(builds)
}

func newVersion(candidate VersionCandidate) version {
	v := version{
		id:     candidate.VersionID,
		order:  candidate.CheckOrder,
		passed: map[int]BuildSet{},
	}

	if candidate.JobID != 0 {
		v.passed[candidate.JobID] = BuildSet{candidate.BuildID: struct{}{}}
	}

	return v
}

func (vs versions) With(candidate VersionCandidate) versions {
	i := sort.Search(len(vs), func(i int) bool {
		return vs[i].order <= candidate.CheckOrder
	})
	if i == len(vs) {
		vs = append(vs, newVersion(candidate))
	}

	if vs[i].id != candidate.VersionID {
		vs = append(vs, version{})
		copy(vs[i+1:], vs[i:])
		vs[i] = newVersion(candidate)
	} else if candidate.JobID != 0 {
		builds, found := vs[i].passed[candidate.JobID]
		if !found {
			builds = BuildSet{}
			vs[i].passed[candidate.JobID] = builds
		}

		builds[candidate.BuildID] = struct{}{}
	}

	return vs
}

func (vs versions) Merge(v version) versions {
	i := sort.Search(len(vs), func(i int) bool {
		return vs[i].order <= v.order
	})
	if i == len(vs) {
		vs = append(vs, v)
	}

	if vs[i].id != v.id {
		vs = append(vs, version{})
		copy(vs[i+1:], vs[i:])
		vs[i] = v
	} else {
		for jobID, vbuilds := range v.passed {
			builds, found := vs[i].passed[jobID]
			if !found {
				vs[i].passed[jobID] = vbuilds
				continue
			}

			for vbuild := range vbuilds {
				builds[vbuild] = struct{}{}
			}
		}
	}

	return vs
}

type constraints []constraintFunc
type constraintFunc func(version) bool

func (cs constraints) check(v version) bool {
	for _, c := range cs {
		if !c(v) {
			log.Println("FAILED", v.order)
			return false
		}
	}

	return true
}

func (cs constraints) and(constraint constraintFunc) constraints {
	ncs := make([]constraintFunc, len(cs)+1)
	copy(ncs, cs)
	ncs[len(cs)] = constraint
	return ncs
}

func (candidates *VersionCandidates) Add(candidate VersionCandidate) {
	candidates.versions = candidates.versions.With(candidate)

	if candidate.JobID != 0 {
		if candidates.buildIDs == nil {
			candidates.buildIDs = map[int]BuildSet{}
		}

		builds, found := candidates.buildIDs[candidate.JobID]
		if !found {
			builds = BuildSet{}
			candidates.buildIDs[candidate.JobID] = builds
		}

		builds[candidate.BuildID] = struct{}{}
	}
}

func (candidates *VersionCandidates) Merge(version version) {
	for jobID, otherBuilds := range version.passed {
		if candidates.buildIDs == nil {
			candidates.buildIDs = map[int]BuildSet{}
		}

		builds, found := candidates.buildIDs[jobID]
		if !found {
			builds = BuildSet{}
			candidates.buildIDs[jobID] = builds
		}

		for build := range otherBuilds {
			builds[build] = struct{}{}
		}
	}

	candidates.versions = candidates.versions.Merge(version)
}

func (candidates VersionCandidates) IsEmpty() bool {
	return len(candidates.versions) == 0
}

func (candidates VersionCandidates) Len() int {
	return len(candidates.versions)
}

func (candidates VersionCandidates) IntersectByVersion(other VersionCandidates) VersionCandidates {
	intersected := VersionCandidates{}

	for _, version := range candidates.versions {
		found := false
		for _, otherVersion := range other.versions {
			if otherVersion.id == version.id {
				found = true
				intersected.Merge(otherVersion)
				break
			}
		}

		if found {
			intersected.Merge(version)
		}
	}

	return intersected
}

func (candidates VersionCandidates) BuildIDs(jobID int) BuildSet {
	builds, found := candidates.buildIDs[jobID]
	if !found {
		builds = BuildSet{}
	}

	return builds
}

func (candidates VersionCandidates) PruneVersionsOfOtherBuildIDs(jobID int, buildIDs BuildSet) VersionCandidates {
	newCandidates := candidates
	newCandidates.constraints = newCandidates.constraints.and(func(v version) bool {
		return v.passedAny(jobID, buildIDs)
	})
	return newCandidates
}

type VersionsIter struct {
	offset      int
	versions    versions
	constraints constraints
}

func (iter *VersionsIter) Next() (int, bool) {
	for i := iter.offset; i < len(iter.versions); i++ {
		v := iter.versions[i]

		iter.offset++

		if !iter.constraints.check(v) {
			continue
		}

		return v.id, true
	}

	return 0, false
}

func (iter *VersionsIter) Peek() (int, bool) {
	for i := iter.offset; i < len(iter.versions); i++ {
		v := iter.versions[i]

		if !iter.constraints.check(v) {
			iter.offset++
			continue
		}

		return v.id, true
	}

	return 0, false
}

func (candidates VersionCandidates) VersionIDs() *VersionsIter {
	return &VersionsIter{
		versions:    candidates.versions,
		constraints: candidates.constraints,
	}
}

func (candidates VersionCandidates) ForVersion(versionID int) VersionCandidates {
	newCandidates := VersionCandidates{}
	for _, version := range candidates.versions {
		if version.id == versionID {
			newCandidates.Merge(version)
			break
		}
	}

	return newCandidates
}

type versionCandidatesSorter struct {
	VersionCandidates []VersionCandidate
}

func (s versionCandidatesSorter) Len() int {
	return len(s.VersionCandidates)
}

func (s versionCandidatesSorter) Swap(i, j int) {
	s.VersionCandidates[i], s.VersionCandidates[j] = s.VersionCandidates[j], s.VersionCandidates[i]
}

func (s versionCandidatesSorter) Less(i, j int) bool {
	return s.VersionCandidates[i].CheckOrder < s.VersionCandidates[j].CheckOrder
}
