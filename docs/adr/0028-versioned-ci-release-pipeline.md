# ADR 0028: Versioned CI And Release Pipeline

- Status: Accepted
- Date: 2026-07-28

## Context

The local gate covers backend, frontend, Compose, manifests and runtime health,
while the real-kind scripts cover fault injection and physical cluster
isolation. Running every real-cluster suite on every change would be slow and
would require privileged infrastructure. Publishing from an untrusted pull
request or from an unversioned working tree would also cross the repository's
credential and evidence boundaries.

KubeSphere separates regular build workflows from scheduled/manual E2E and
image workflows. KRM and Ratel do not provide a comparable local workflow to
reuse. This project adopts the separation pattern, not their source or
credentials, and applies a smaller release surface suitable for this platform.

## Decision

Add three GitHub Actions workflows and one dependency-update policy:

- `.github/workflows/ci.yml` runs on `main`, pull requests, manual dispatch and
  reusable `workflow_call`. It uses read-only repository permission, pinned
  official action revisions and versioned Go/Node/pnpm/kubectl inputs. Backend,
  frontend and manifest checks precede a real Compose build/runtime check with
  random ephemeral credentials and unconditional teardown.
- `.github/workflows/release.yml` accepts a validated `vX.Y.Z`-style version,
  calls the complete CI workflow and builds versioned backend/frontend
  linux-amd64 OCI archives, a source archive, OpenAPI, dependency-license
  inventory, metadata and `SHA256SUMS`. Manual dispatch is package-only. Only
  an existing tag may create a GitHub Release, using `--verify-tag`.
- `.github/workflows/real-kind-e2e.yml` runs weekly or manually on a dedicated
  `[self-hosted, windows, x64, aiops-kind]` runner. It runs only the disposable
  diagnosis, fleet and global-search suites, does not cancel an active run and
  uploads only ignored sanitized JSON evidence.
- `.github/dependabot.yml` proposes grouped weekly updates for Actions, Go
  modules and frontend packages. Action upgrades must preserve commit-SHA
  pinning after review.

The workflow contract test parses every workflow/configuration YAML and checks
required triggers, permissions, runners, cleanup and prohibited markers. No
workflow uses `pull_request_target`, repository secrets in PR validation,
unreviewed `docker push`, retained-demo mode or a generic cluster command.

## Consequences

- A normal pull request needs no platform or Kubernetes credential.
- Real-kind execution requires a separately administered runner with Docker
  Desktop Linux containers, kubectl and the reviewed bundled kind executable.
  The runner must not host production workloads or long-lived platform data.
- The release workflow produces reviewable packages but does not push to a
  registry. Registry promotion, signing and provenance attestation require a
  later explicit trust and credential decision.
- Formal branch protection should require Backend, Frontend, Manifests and
  Compose runtime jobs after the initial baseline reaches a remote repository.
- No artifact may claim a revision or release until the human-reviewed initial
  commit exists. Creating that commit and release tag remains a human action.
