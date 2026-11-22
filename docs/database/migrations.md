# Database migrations

Provision a dedicated empty database for a new installation. Run `cryol-cli migrate` with its migration credentials, then run `cryol-cli schema-check` with runtime credentials. The service verifies the schema and installation row before starting. It leaves schema changes to the CLI.

Goose records migration versions in `goose_db_version` and takes a PostgreSQL session advisory lock during the upgrade. Keep migrations in increasing order, preserve published files, and add a new migration for each change. Take and verify a backup before applying a production upgrade. The CLI exposes forward migration only.

Update the runtime schema version in `internal/database/database.go` whenever a migration is added. To regenerate typed queries, install sqlc 1.31.0 from its official release and run `sqlc generate` from `apps/server`.

For a separately provisioned runtime role, grant connection to the Cryol database, usage on the application schema and SELECT on `installation_info` and `goose_db_version`. Run the migration CLI only against an empty database dedicated to Cryol or a verified Cryol database. The current CLI relies on the operator to select and verify that database.
