// This file was generated by counterfeiter
package resourcefakes

import (
	"os"
	"sync"

	"github.com/concourse/atc"
	"github.com/concourse/atc/resource"
	"github.com/concourse/atc/worker"
	"code.cloudfoundry.org/lager"
)

type FakeFetcher struct {
	FetchStub        func(logger lager.Logger, session resource.Session, tags atc.Tags, teamID int, resourceTypes atc.ResourceTypes, cacheIdentifier resource.CacheIdentifier, metadata resource.Metadata, imageFetchingDelegate worker.ImageFetchingDelegate, resourceOptions resource.ResourceOptions, signals <-chan os.Signal, ready chan<- struct{}) (resource.FetchSource, error)
	fetchMutex       sync.RWMutex
	fetchArgsForCall []struct {
		logger                lager.Logger
		session               resource.Session
		tags                  atc.Tags
		teamID                int
		resourceTypes         atc.ResourceTypes
		cacheIdentifier       resource.CacheIdentifier
		metadata              resource.Metadata
		imageFetchingDelegate worker.ImageFetchingDelegate
		resourceOptions       resource.ResourceOptions
		signals               <-chan os.Signal
		ready                 chan<- struct{}
	}
	fetchReturns struct {
		result1 resource.FetchSource
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeFetcher) Fetch(logger lager.Logger, session resource.Session, tags atc.Tags, teamID int, resourceTypes atc.ResourceTypes, cacheIdentifier resource.CacheIdentifier, metadata resource.Metadata, imageFetchingDelegate worker.ImageFetchingDelegate, resourceOptions resource.ResourceOptions, signals <-chan os.Signal, ready chan<- struct{}) (resource.FetchSource, error) {
	fake.fetchMutex.Lock()
	fake.fetchArgsForCall = append(fake.fetchArgsForCall, struct {
		logger                lager.Logger
		session               resource.Session
		tags                  atc.Tags
		teamID                int
		resourceTypes         atc.ResourceTypes
		cacheIdentifier       resource.CacheIdentifier
		metadata              resource.Metadata
		imageFetchingDelegate worker.ImageFetchingDelegate
		resourceOptions       resource.ResourceOptions
		signals               <-chan os.Signal
		ready                 chan<- struct{}
	}{logger, session, tags, teamID, resourceTypes, cacheIdentifier, metadata, imageFetchingDelegate, resourceOptions, signals, ready})
	fake.recordInvocation("Fetch", []interface{}{logger, session, tags, teamID, resourceTypes, cacheIdentifier, metadata, imageFetchingDelegate, resourceOptions, signals, ready})
	fake.fetchMutex.Unlock()
	if fake.FetchStub != nil {
		return fake.FetchStub(logger, session, tags, teamID, resourceTypes, cacheIdentifier, metadata, imageFetchingDelegate, resourceOptions, signals, ready)
	} else {
		return fake.fetchReturns.result1, fake.fetchReturns.result2
	}
}

func (fake *FakeFetcher) FetchCallCount() int {
	fake.fetchMutex.RLock()
	defer fake.fetchMutex.RUnlock()
	return len(fake.fetchArgsForCall)
}

func (fake *FakeFetcher) FetchArgsForCall(i int) (lager.Logger, resource.Session, atc.Tags, int, atc.ResourceTypes, resource.CacheIdentifier, resource.Metadata, worker.ImageFetchingDelegate, resource.ResourceOptions, <-chan os.Signal, chan<- struct{}) {
	fake.fetchMutex.RLock()
	defer fake.fetchMutex.RUnlock()
	return fake.fetchArgsForCall[i].logger, fake.fetchArgsForCall[i].session, fake.fetchArgsForCall[i].tags, fake.fetchArgsForCall[i].teamID, fake.fetchArgsForCall[i].resourceTypes, fake.fetchArgsForCall[i].cacheIdentifier, fake.fetchArgsForCall[i].metadata, fake.fetchArgsForCall[i].imageFetchingDelegate, fake.fetchArgsForCall[i].resourceOptions, fake.fetchArgsForCall[i].signals, fake.fetchArgsForCall[i].ready
}

func (fake *FakeFetcher) FetchReturns(result1 resource.FetchSource, result2 error) {
	fake.FetchStub = nil
	fake.fetchReturns = struct {
		result1 resource.FetchSource
		result2 error
	}{result1, result2}
}

func (fake *FakeFetcher) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.fetchMutex.RLock()
	defer fake.fetchMutex.RUnlock()
	return fake.invocations
}

func (fake *FakeFetcher) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ resource.Fetcher = new(FakeFetcher)
