-- +goose Up
ALTER TABLE asset
    ADD COLUMN manufacturer text NOT NULL DEFAULT '' CHECK (char_length(manufacturer) <= 120),
    ADD COLUMN model text NOT NULL DEFAULT '' CHECK (char_length(model) <= 120),
    ADD COLUMN notes text NOT NULL DEFAULT '' CHECK (char_length(notes) <= 4000),
    ADD COLUMN purchase_date date,
    ADD COLUMN archived_at timestamptz,
    ADD COLUMN revision integer NOT NULL DEFAULT 1 CHECK (revision > 0);

CREATE INDEX asset_active_created_idx ON asset (created_at DESC, id DESC) WHERE archived_at IS NULL;
INSERT INTO app_permission VALUES ('assets.update'), ('assets.archive');
INSERT INTO role_permission VALUES ('system-administrator','assets.update'), ('system-administrator','assets.archive');
CREATE INDEX audit_log_event_id_idx ON audit_log (event, id DESC);
