-- +goose Up
CREATE TABLE agent_enrollment (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_digest bytea UNIQUE NOT NULL CHECK (octet_length(token_digest) = 32),
    created_by uuid NOT NULL REFERENCES app_user,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz
);
CREATE TABLE inventory_device (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id uuid UNIQUE REFERENCES asset ON DELETE RESTRICT,
    hostname text NOT NULL DEFAULT '',
    os_family text NOT NULL DEFAULT '',
    architecture text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE endpoint_agent (
    id uuid PRIMARY KEY,
    device_id uuid UNIQUE NOT NULL REFERENCES inventory_device ON DELETE RESTRICT,
    token_digest bytea UNIQUE NOT NULL CHECK (octet_length(token_digest) = 32),
    version text NOT NULL DEFAULT '',
    last_seen timestamptz NOT NULL DEFAULT now(),
    last_inventory_at timestamptz,
    revoked_at timestamptz
);
CREATE TABLE inventory_snapshot (
    agent_id uuid NOT NULL REFERENCES endpoint_agent ON DELETE RESTRICT,
    run_id uuid NOT NULL,
    payload jsonb NOT NULL,
    scanned_at timestamptz NOT NULL,
    digest bytea NOT NULL CHECK (octet_length(digest) = 32),
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, run_id)
);
CREATE INDEX inventory_snapshot_recent_idx ON inventory_snapshot (agent_id, scanned_at DESC, received_at DESC);
CREATE INDEX inventory_software_recent_idx ON inventory_snapshot (agent_id, scanned_at DESC, received_at DESC)
    WHERE payload->>'softwareStatus'='complete';
CREATE INDEX endpoint_agent_seen_idx ON endpoint_agent (last_seen DESC, id);
CREATE INDEX inventory_device_hostname_idx ON inventory_device (lower(hostname), id);
INSERT INTO app_permission VALUES ('agents.read'), ('agents.enroll'), ('agents.revoke'), ('devices.manage'), ('hardware.read'), ('software.read');
INSERT INTO role_permission SELECT 'system-administrator', code FROM app_permission
WHERE code IN ('agents.read','agents.enroll','agents.revoke','devices.manage','hardware.read','software.read');
