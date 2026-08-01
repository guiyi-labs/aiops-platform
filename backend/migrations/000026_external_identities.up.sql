-- M36: Production OIDC and MFA.
-- Adds the external_identities table used for administrator-owned, immutable
-- subject prelinking (ADR 0052). A provider subject is bound to exactly one
-- local user before that user can authenticate through OIDC. Automatic email
-- linking is forbidden; this table is the only link between an OIDC subject
-- and a local account.

CREATE TABLE IF NOT EXISTS external_identities (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issuer VARCHAR(512) NOT NULL,
    subject VARCHAR(512) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- One provider subject maps to at most one local user (immutable prelink).
    CONSTRAINT external_identities_issuer_subject_unique UNIQUE (issuer, subject),
    -- A local user may be prelinked to the same provider at most once.
    CONSTRAINT external_identities_user_issuer_subject_unique UNIQUE (user_id, issuer, subject)
);

CREATE INDEX IF NOT EXISTS external_identities_user_idx ON external_identities (user_id);
CREATE INDEX IF NOT EXISTS external_identities_issuer_subject_idx ON external_identities (issuer, subject);
