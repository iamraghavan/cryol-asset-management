-- +goose Up
CREATE TABLE app_user (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL UNIQUE CHECK (username ~ '^[a-z0-9][a-z0-9._-]{2,63}$'),
    password_salt bytea NOT NULL CHECK (octet_length(password_salt) = 16),
    password_digest bytea NOT NULL CHECK (octet_length(password_digest) = 32),
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app_role (code text PRIMARY KEY);
CREATE TABLE app_permission (code text PRIMARY KEY);
CREATE TABLE role_permission (
    role_code text REFERENCES app_role ON DELETE CASCADE,
    permission_code text REFERENCES app_permission ON DELETE CASCADE,
    PRIMARY KEY (role_code, permission_code)
);
CREATE TABLE user_role (
    user_id uuid REFERENCES app_user ON DELETE CASCADE,
    role_code text REFERENCES app_role ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_code)
);

INSERT INTO app_role VALUES ('system-administrator');
INSERT INTO app_permission VALUES ('settings.read'), ('settings.manage');
INSERT INTO role_permission SELECT 'system-administrator', code FROM app_permission;

CREATE TABLE user_session (
    token_digest bytea PRIMARY KEY CHECK (octet_length(token_digest) = 32),
    user_id uuid NOT NULL REFERENCES app_user ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);
CREATE INDEX user_session_expiry_idx ON user_session(expires_at);

CREATE TABLE organization_profile (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    address text NOT NULL DEFAULT '',
    contact text NOT NULL DEFAULT '',
    timezone text NOT NULL DEFAULT 'UTC',
    currency text NOT NULL DEFAULT 'USD' CHECK (currency ~ '^[A-Z]{3}$'),
    financial_year_month smallint NOT NULL DEFAULT 1 CHECK (financial_year_month BETWEEN 1 AND 12),
    asset_prefix text NOT NULL DEFAULT 'CRY' CHECK (asset_prefix ~ '^[A-Z0-9-]{1,16}$'),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id uuid REFERENCES app_user ON DELETE RESTRICT,
    event text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now()
);
