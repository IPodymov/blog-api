-- Platform admins are global accounts outside every site; they manage all sites through /api/admin/v1.
-- The first admin is created by the operator with cmd/admin, never through the public API.
CREATE TABLE platform_admins (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 email text NOT NULL UNIQUE CHECK (email=lower(email) AND length(email) BETWEEN 3 AND 254),
 password_hash text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE admin_sessions (
 token_hash text PRIMARY KEY,
 admin_id bigint NOT NULL REFERENCES platform_admins ON DELETE CASCADE,
 expires_at timestamptz NOT NULL
);
CREATE INDEX admin_sessions_admin ON admin_sessions(admin_id);
CREATE INDEX admin_sessions_expiry ON admin_sessions(expires_at);

-- An invite lets one person register on a site with a preset role; only the SHA-256 of the token is stored.
CREATE TABLE invites (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 token_hash text NOT NULL UNIQUE,
 site_id bigint NOT NULL REFERENCES sites ON DELETE CASCADE,
 role text NOT NULL CHECK(role IN ('user','author','admin')),
 email text CHECK(email IS NULL OR (email=lower(email) AND length(email) BETWEEN 3 AND 254)),
 created_by bigint REFERENCES platform_admins ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 used_by bigint REFERENCES users ON DELETE SET NULL,
 used_at timestamptz
);
CREATE INDEX invites_site ON invites(site_id,id DESC);
