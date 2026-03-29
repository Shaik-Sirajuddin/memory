CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS account (
  uid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255),
  account_type VARCHAR(10) NOT NULL CHECK (account_type IN ('user', 'team')),
  organization VARCHAR(255),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_email ON account(email);
CREATE INDEX IF NOT EXISTS idx_account_org ON account(organization);

CREATE TABLE IF NOT EXISTS oauth (
  uid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_uid UUID NOT NULL REFERENCES account(uid) ON DELETE CASCADE,
  provider VARCHAR(20) NOT NULL CHECK (provider IN ('google', 'github')),
  provider_uid VARCHAR(255) NOT NULL,
  access_token TEXT,
  refresh_token TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (provider, provider_uid)
);

CREATE TABLE IF NOT EXISTS service_account (
  uid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_uid UUID NOT NULL REFERENCES account(uid) ON DELETE CASCADE,
  name VARCHAR(255) NOT NULL,
  access_key VARCHAR(255) UNIQUE NOT NULL,
  secret_key TEXT,
  expiry_date TIMESTAMPTZ,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS account_scope (
  uid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_account_uid UUID NOT NULL REFERENCES service_account(uid) ON DELETE CASCADE,
  root VARCHAR(10) NOT NULL CHECK (root IN ('user', 'team')),
  path VARCHAR(500) NOT NULL,
  scope VARCHAR(10) NOT NULL CHECK (scope IN ('read', 'memory', 'write')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scope_sa ON account_scope(service_account_uid);

