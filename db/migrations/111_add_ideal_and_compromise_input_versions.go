package migrations

import "github.com/BurntSushi/migration"

func AddIdealAndCompromiseInputVersions(tx migration.LimitedTx) error {
	_, err := tx.Exec(`
	CREATE TABLE ideal_input_versions (
		id serial PRIMARY KEY,
		job_id integer NOT NULL,
		CONSTRAINT ideal_input_versions_job_id_fkey
			FOREIGN KEY (job_id)
			REFERENCES jobs (id)
			ON DELETE CASCADE,
		input_name text NOT NULL,
		CONSTRAINT ideal_input_versions_unique_job_id_input_name
			UNIQUE (job_id, input_name),
		version_id integer NOT NULL,
		CONSTRAINT compromise_input_versions_version_id_fkey
			FOREIGN KEY (version_id)
			REFERENCES versioned_resources (id)
			ON DELETE CASCADE,
		first_occurrence bool NOT NULL
	)
	`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
	CREATE TABLE compromise_input_versions (
		id serial PRIMARY KEY,
		job_id integer NOT NULL,
		CONSTRAINT compromise_input_versions_job_id_fkey
			FOREIGN KEY (job_id)
			REFERENCES jobs (id)
			ON DELETE CASCADE,
		input_name text NOT NULL,
		CONSTRAINT compromise_input_versions_unique_job_id_input_name
			UNIQUE (job_id, input_name),
		version_id integer NOT NULL,
		CONSTRAINT compromise_input_versions_version_id_fkey
			FOREIGN KEY (version_id)
			REFERENCES versioned_resources (id)
			ON DELETE CASCADE,
		first_occurrence bool NOT NULL
	)
	`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
	ALTER TABLE jobs
	ADD COLUMN resource_checking_expires_at timestamp NOT NULL DEFAULT 'epoch',
	ADD COLUMN inputs_determined bool NOT NULL DEFAULT false
	`)
	return err
}
