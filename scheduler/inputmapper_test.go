package scheduler_test

// import (
// 	"github.com/concourse/atc"
// 	"github.com/concourse/atc/db/algorithm"
// 	. "github.com/concourse/atc/scheduler"
// 	"github.com/concourse/atc/scheduler/schedulerfakes"
//
// 	. "github.com/onsi/ginkgo"
// 	. "github.com/onsi/gomega"
// )
//
// var _ = Describe("Inputmapper", func() {
// 	var (
// 		fakeDB      *schedulerfakes.FakeInputMapperDB
// 		inputMapper *InputMapper
// 	)
// 	BeforeEach(func() {
// 		inputMapper = &InputMapper{
// 			DB: fakeDB,
// 		}
// 	})
//
// 	Describe("SaveNextInputMapping", func() {
// 		var jobConfig atc.JobConfig
//
// 		BeforeEach(func() {
// 			jobConfig = atc.JobConfig{
// 				Name: "some-job",
//
// 				Serial: true,
//
// 				Plan: atc.PlanSequence{
// 					{
// 						Get:      "some-input",
// 						Resource: "some-resource",
// 						Params:   atc.Params{"some": "params"},
// 						Trigger:  true,
// 					},
// 					{
// 						Get:      "some-other-input",
// 						Resource: "some-other-resource",
// 						Params:   atc.Params{"some": "other-params"},
// 						Trigger:  true,
// 					},
// 				},
// 			}
// 		})
//
// 		Context("when there are algorithm input configs", func() {
// 			BeforeEach(func() {
// 				fakeDB.GetAlgorithmInputConfigsReturns(algorithm.InputConfigs{}, nil)
// 			})
//
// 			It("saves singleton mapping as independent input mapping", func() {
//
// 			})
// 		})
// 	})
// })
