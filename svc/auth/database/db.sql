-- account: root identity, email is PK concept, org auto-detected from domain
CREATE TABLE account (
  uid           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email         VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255),                          -- null for OAuth-only accounts
  account_type  VARCHAR(10)  NOT NULL CHECK (account_type IN ('user','team')),
  organization  VARCHAR(255),                          -- derived from email domain on signup
  created_at    TIMESTAMPTZ  DEFAULT NOW(),
  updated_at    TIMESTAMPTZ  DEFAULT NOW()
);
CREATE INDEX idx_account_email ON account(email);
CREATE INDEX idx_account_org   ON account(organization);

-- oauth: one account can link both google + github
CREATE TABLE oauth (
  uid          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_uid  UUID NOT NULL REFERENCES account(uid) ON DELETE CASCADE,
  provider     VARCHAR(20)  NOT NULL CHECK (provider IN ('google','github')),
  provider_uid VARCHAR(255) NOT NULL,
  access_token TEXT,
  refresh_token TEXT,
  created_at   TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE (provider, provider_uid)
);

-- service_account: secret_key is nullable; null means no HMAC required
CREATE TABLE service_account (
  uid          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_uid  UUID NOT NULL REFERENCES account(uid) ON DELETE CASCADE,
  name         VARCHAR(255) NOT NULL,
  access_key   VARCHAR(255) UNIQUE NOT NULL,   -- public identifier (e.g. "sa_abc123")
  secret_key   VARCHAR(255),                   -- bcrypt hash; null = secret-less SA
  expiry_date  TIMESTAMPTZ,
  is_active    BOOLEAN DEFAULT TRUE,
  created_at   TIMESTAMPTZ DEFAULT NOW(),
  updated_at   TIMESTAMPTZ DEFAULT NOW()
);

-- account_scope: RBAC rules per service account
CREATE TABLE account_scope (
  uid                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_account_uid  UUID NOT NULL REFERENCES service_account(uid) ON DELETE CASCADE,
  root                 VARCHAR(10) NOT NULL CHECK (root IN ('user','team')),
  path                 VARCHAR(500) NOT NULL,
  scope                VARCHAR(10)  NOT NULL CHECK (scope IN ('read','memory','write')),
  created_at           TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_scope_sa ON account_scope(service_account_uid);