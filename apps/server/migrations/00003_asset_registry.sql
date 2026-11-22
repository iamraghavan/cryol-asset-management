-- +goose Up
CREATE SEQUENCE asset_number_seq;
CREATE TABLE asset (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_number text UNIQUE NOT NULL,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    serial_number text NOT NULL DEFAULT '' CHECK (char_length(serial_number) <= 120),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX asset_created_idx ON asset (created_at DESC, id DESC);
INSERT INTO app_permission VALUES ('assets.read'), ('assets.create');
INSERT INTO role_permission VALUES ('system-administrator','assets.read'), ('system-administrator','assets.create');
