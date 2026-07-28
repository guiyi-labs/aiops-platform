# OIDC and MFA Readiness Runbook

This runbook evaluates a proposed identity-provider contract without enabling
login or contacting the provider. A passing report is a prerequisite, not an
SSO deployment approval.

## Required Decisions

Identity, security and application owners must jointly approve:

- the canonical issuer and client registration;
- exact redirect URIs and required scopes;
- immutable subject, username, display-name and group claims;
- accepted `acr` or `amr` values that prove MFA;
- administrator-owned subject prelinking and the prohibition on automatic
  email linking;
- maximum session age, reauthentication interval, identity-disable revocation
  and local/provider logout behavior;
- break-glass ownership, offline secret storage, account count and drill
  interval.

Start from `docs/security/identity-readiness-policy.example.json`. Replace every
`REQUIRED_*` marker through an approved change. The file is policy metadata;
never put a client secret, access token, refresh token, private key or user
claim sample in it.

## Capture Inputs

Capture `/.well-known/openid-configuration` and the referenced JWKS through the
organization's approved administrative channel. Review their origin out of
band, store working copies outside Git and do not rewrite issuer or endpoint
values to make a check pass. Each JSON file must be at most 1 MiB.

## Evaluate

Run the production image with no network and read-only inputs:

```powershell
docker run --rm --network none `
  --mount type=bind,source=C:\secure\identity-review,target=/review,readonly `
  --entrypoint /app/identity-readiness aiops-platform-backend:reviewed `
  --policy-file=/review/policy.json `
  --discovery-file=/review/openid-configuration.json `
  --jwks-file=/review/jwks.json
```

Exit code `0` and `ready: true` mean all checks passed. Exit code `1` means an
input is malformed or one or more security checks failed; exit code `2` means
required command arguments are missing. Reports contain no secrets and may be
attached to a security review only after checking organizational metadata.

The repository gate uses synthetic data:

```powershell
.\scripts\e2e-identity-readiness.ps1
```

It runs with Docker networking disabled, accepts 14 checks, rejects issuer and
PKCE downgrades, rejects missing MFA and automatic email linking, deletes all
temporary snapshots/image material and writes sanitized evidence under
`.artifacts/identity-readiness`.

## Production Integration Boundary

Do not enable OIDC from this report alone. The implementation phase must still
verify `state`, nonce and PKCE; validate issuer, audience, signature, expiry and
MFA evidence; support bounded JWKS rollover; persist provider plus immutable
subject; implement explicit prelinking; revoke local sessions on disable;
audit login/link/logout failures without tokens; and test browser flows against
an isolated provider. Keep reviewed local break-glass access available until
provider outage and recovery drills pass.
