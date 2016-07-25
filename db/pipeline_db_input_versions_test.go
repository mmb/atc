package db_test

import (
	"time"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/algorithm"
	"github.com/lib/pq"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Input Versions", func() {
	var dbConn db.Conn
	var listener *pq.Listener

	var pipelineDBFactory db.PipelineDBFactory
	var sqlDB *db.SQLDB
	var pipelineDB db.PipelineDB
	var pipelineDB2 db.PipelineDB
	var versions db.SavedVersionedResources

	BeforeEach(func() {
		postgresRunner.Truncate()

		dbConn = db.Wrap(postgresRunner.Open())

		listener = pq.NewListener(postgresRunner.DataSourceName(), time.Second, time.Minute, nil)
		Eventually(listener.Ping, 5*time.Second).ShouldNot(HaveOccurred())
		bus := db.NewNotificationsBus(listener, dbConn)

		sqlDB = db.NewSQL(dbConn, bus)
		pipelineDBFactory = db.NewPipelineDBFactory(dbConn, bus, sqlDB)

		team, err := sqlDB.SaveTeam(db.Team{Name: "some-team"})
		Expect(err).NotTo(HaveOccurred())

		resourceConfig := atc.ResourceConfig{
			Name: "some-resource",
			Type: "some-type",
		}

		config := atc.Config{
			Jobs: atc.JobConfigs{
				{
					Name: "some-job",
				},
				{
					Name: "some-other-job",
				},
			},
			Resources: atc.ResourceConfigs{resourceConfig},
		}

		_, _, err = sqlDB.SaveConfig(team.Name, "some-pipeline", config, 0, db.PipelineUnpaused)
		Expect(err).NotTo(HaveOccurred())

		pipelineDB, err = pipelineDBFactory.BuildWithTeamNameAndName(team.Name, "some-pipeline")
		Expect(err).NotTo(HaveOccurred())

		err = pipelineDB.SaveResourceVersions(
			resourceConfig,
			[]atc.Version{
				{"version": "v1"},
				{"version": "v2"},
				{"version": "v3"},
			},
		)
		Expect(err).NotTo(HaveOccurred())

		// save metadata for v1
		build, err := pipelineDB.CreateJobBuild("some-job", false)
		Expect(err).ToNot(HaveOccurred())
		_, err = pipelineDB.SaveBuildInput(build.ID, db.BuildInput{
			Name: "some-input",
			VersionedResource: db.VersionedResource{
				Resource:   "some-resource",
				Type:       "some-type",
				Version:    db.Version{"version": "v1"},
				Metadata:   []db.MetadataField{{Name: "name1", Value: "value1"}},
				PipelineID: pipelineDB.GetPipelineID(),
			},
			FirstOccurrence: true,
		})
		Expect(err).NotTo(HaveOccurred())

		reversions, _, found, err := pipelineDB.GetResourceVersions("some-resource", db.Page{Limit: 3})
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())

		versions = []db.SavedVersionedResource{reversions[2], reversions[1], reversions[0]}

		_, _, err = sqlDB.SaveConfig(team.Name, "some-pipeline-2", config, 1, db.PipelineUnpaused)
		Expect(err).NotTo(HaveOccurred())

		pipelineDB2, err = pipelineDBFactory.BuildWithTeamNameAndName(team.Name, "some-pipeline-2")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := dbConn.Close()
		Expect(err).NotTo(HaveOccurred())

		err = listener.Close()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("can save and get ideal input versions", func() {
		It("gets ideal input versions for the given job name", func() {
			inputVersions := algorithm.InputMapping{
				"some-input-1": algorithm.InputVersion{
					VersionID:       versions[0].ID,
					FirstOccurrence: false,
				},
				"some-input-2": algorithm.InputVersion{
					VersionID:       versions[1].ID,
					FirstOccurrence: true,
				},
			}
			err := pipelineDB.SaveIdealInputVersions(inputVersions, "some-job")
			Expect(err).NotTo(HaveOccurred())

			pipeline2InputVersions := algorithm.InputMapping{
				"some-input-3": algorithm.InputVersion{
					VersionID:       versions[2].ID,
					FirstOccurrence: false,
				},
			}
			err = pipelineDB2.SaveIdealInputVersions(pipeline2InputVersions, "some-job")
			Expect(err).NotTo(HaveOccurred())

			buildInputs := []db.BuildInput{
				{
					Name:              "some-input-1",
					VersionedResource: versions[0].VersionedResource,
					FirstOccurrence:   false,
				},
				{
					Name:              "some-input-2",
					VersionedResource: versions[1].VersionedResource,
					FirstOccurrence:   true,
				},
			}

			actualBuildInputs, err := pipelineDB.GetIdealBuildInputs("some-job")
			Expect(err).NotTo(HaveOccurred())

			Expect(actualBuildInputs).To(ConsistOf(buildInputs))

			By("updating the set of ideal input versions")
			inputVersions2 := algorithm.InputMapping{
				"some-input-2": algorithm.InputVersion{
					VersionID:       versions[2].ID,
					FirstOccurrence: false,
				},
				"some-input-3": algorithm.InputVersion{
					VersionID:       versions[2].ID,
					FirstOccurrence: true,
				},
			}
			err = pipelineDB.SaveIdealInputVersions(inputVersions2, "some-job")
			Expect(err).NotTo(HaveOccurred())

			buildInputs2 := []db.BuildInput{
				{
					Name:              "some-input-2",
					VersionedResource: versions[2].VersionedResource,
					FirstOccurrence:   false,
				},
				{
					Name:              "some-input-3",
					VersionedResource: versions[2].VersionedResource,
					FirstOccurrence:   true,
				},
			}

			actualBuildInputs2, err := pipelineDB.GetIdealBuildInputs("some-job")
			Expect(err).NotTo(HaveOccurred())

			Expect(actualBuildInputs2).To(ConsistOf(buildInputs2))

			By("deleting all ideal input versions for the job")
			err = pipelineDB.SaveIdealInputVersions(nil, "some-job")
			Expect(err).NotTo(HaveOccurred())

			actualBuildInputs3, err := pipelineDB.GetIdealBuildInputs("some-job")
			Expect(actualBuildInputs3).To(BeEmpty())
		})
	})

	Describe("can save and get compromise input versions", func() {
		It("gets compromise input versions for the given job name", func() {
			inputVersions := algorithm.InputMapping{
				"some-input-1": algorithm.InputVersion{
					VersionID:       versions[0].ID,
					FirstOccurrence: false,
				},
				"some-input-2": algorithm.InputVersion{
					VersionID:       versions[1].ID,
					FirstOccurrence: true,
				},
			}
			err := pipelineDB.SaveCompromiseInputVersions(inputVersions, "some-job")
			Expect(err).NotTo(HaveOccurred())

			pipeline2InputVersions := algorithm.InputMapping{
				"some-input-3": algorithm.InputVersion{
					VersionID:       versions[2].ID,
					FirstOccurrence: false,
				},
			}
			err = pipelineDB2.SaveCompromiseInputVersions(pipeline2InputVersions, "some-job")
			Expect(err).NotTo(HaveOccurred())

			buildInputs := []db.BuildInput{
				{
					Name:              "some-input-1",
					VersionedResource: versions[0].VersionedResource,
					FirstOccurrence:   false,
				},
				{
					Name:              "some-input-2",
					VersionedResource: versions[1].VersionedResource,
					FirstOccurrence:   true,
				},
			}

			actualBuildInputs, err := pipelineDB.GetCompromiseBuildInputs("some-job")
			Expect(err).NotTo(HaveOccurred())

			Expect(actualBuildInputs).To(ConsistOf(buildInputs))

			By("updating the set of compromise input versions")
			inputVersions2 := algorithm.InputMapping{
				"some-input-2": algorithm.InputVersion{
					VersionID:       versions[2].ID,
					FirstOccurrence: false,
				},
				"some-input-3": algorithm.InputVersion{
					VersionID:       versions[2].ID,
					FirstOccurrence: true,
				},
			}
			err = pipelineDB.SaveCompromiseInputVersions(inputVersions2, "some-job")
			Expect(err).NotTo(HaveOccurred())

			buildInputs2 := []db.BuildInput{
				{
					Name:              "some-input-2",
					VersionedResource: versions[2].VersionedResource,
					FirstOccurrence:   false,
				},
				{
					Name:              "some-input-3",
					VersionedResource: versions[2].VersionedResource,
					FirstOccurrence:   true,
				},
			}

			actualBuildInputs2, err := pipelineDB.GetCompromiseBuildInputs("some-job")
			Expect(err).NotTo(HaveOccurred())

			Expect(actualBuildInputs2).To(ConsistOf(buildInputs2))

			By("deleting all compromise input versions for the job")
			err = pipelineDB.SaveCompromiseInputVersions(nil, "some-job")
			Expect(err).NotTo(HaveOccurred())

			actualBuildInputs3, err := pipelineDB.GetCompromiseBuildInputs("some-job")
			Expect(actualBuildInputs3).To(BeEmpty())
		})
	})
})
