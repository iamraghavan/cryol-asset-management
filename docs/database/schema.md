# Database schema

The foundation creates `installation_info`. A boolean primary key constrained to true permits one row. The row has a generated UUID and a timezone-aware creation timestamp. Goose maintains its own migration history in `goose_db_version`.

Migration 2 adds users, roles, permissions, database-backed sessions, the singleton organization profile and audit events. Session rows store token digests, and password salts and digests have database length constraints.

Migration 4 adds manufacturer, model, notes, purchase date, archive timestamp and a revision to the existing asset table. Asset updates, archive and restore compare the supplied revision and increment it on success. Each committed change writes an audit event in the same statement. The asset history uses the indexed event key and returns the latest 25 events. Substring search is parameterized and bounded by the request and page limits. Its large-inventory performance has yet to be benchmarked.

Migration 5 adds single-use enrollment records, endpoint agents, discovered devices and versioned snapshots. Each endpoint has a UUID and an independent hashed credential. Devices may reference one manually managed asset through a unique foreign key. The agent UUID and run UUID form the snapshot idempotency key. Hardware and software queries choose their latest scan by scan time. Software queries skip unavailable collectors and continue returning the most recent complete report.
