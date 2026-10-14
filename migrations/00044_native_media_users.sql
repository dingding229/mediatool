-- +goose Up
-- Logical v1 libraries; no physical file movement or existing STRM rewriting.
CREATE TABLE native_media_libraries (
 id text PRIMARY KEY,
 name text NOT NULL,
 media_type text NOT NULL UNIQUE CHECK (media_type IN ('movie','series'))
);
INSERT INTO native_media_libraries(id,name,media_type) VALUES
 ('movies','电影','movie'),('series','剧集','series');
CREATE TABLE media_users (
 id uuid PRIMARY KEY,
 username text NOT NULL UNIQUE,
 display_name text NOT NULL,
 password_hash text NOT NULL,
 enabled boolean NOT NULL DEFAULT true,
 must_change_password boolean NOT NULL DEFAULT true,
 credential_version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE media_user_library_grants (
 user_id uuid NOT NULL REFERENCES media_users(id) ON DELETE CASCADE,
 library_id text NOT NULL REFERENCES native_media_libraries(id),
 can_play boolean NOT NULL DEFAULT false,
 PRIMARY KEY(user_id,library_id)
);
CREATE TABLE media_user_sessions (
 token_digest bytea PRIMARY KEY CHECK(octet_length(token_digest)=32),
 user_id uuid NOT NULL REFERENCES media_users(id) ON DELETE CASCADE,
 credential_version bigint NOT NULL,
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX media_user_sessions_user ON media_user_sessions(user_id);
CREATE INDEX media_user_sessions_expiry ON media_user_sessions(expires_at);
CREATE TABLE media_user_auth_limits (
 scope_digest bytea PRIMARY KEY,
 window_start timestamptz NOT NULL,
 attempts integer NOT NULL
);
-- +goose Down
DROP TABLE media_user_auth_limits,media_user_sessions,media_user_library_grants,media_users,native_media_libraries;
