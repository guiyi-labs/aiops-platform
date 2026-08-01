# Contributing to AIOps Platform

Thank you for your interest in contributing to the Kubernetes Multi-Cluster AIOps Platform! This document outlines the process for contributing to the project.

## Prerequisites

- Go 1.26+ for backend development
- Node.js 22+ and pnpm 11+ for frontend development
- Docker and Docker Compose for local development
- kubectl and kind for E2E testing

## Getting Started

1. Fork the repository and clone your fork
2. Create a new branch for your changes:
   ```bash
   git checkout -b feat/your-feature-name
   ```
3. Make your changes, following the existing code style and conventions
4. Run the test suite:
   ```bash
   cd backend && go test -p=1 -count=1 ./...
   cd ../frontend && pnpm lint && pnpm typecheck && pnpm test -- --run
   ```
5. Ensure CI gates pass locally:
   ```bash
   gofmt -l .   # no unformatted files
   go vet ./...  # no vet warnings
   ```
6. Commit and push your changes
7. Open a pull request against the `main` branch

## Code Conventions

- **Backend (Go)**: Follow standard Go conventions. Run `gofmt` before committing. Use `gofumpt` for extra strict formatting if preferred.
- **Frontend (Vue/TypeScript)**: Follow ESLint rules. Run `pnpm lint` and `pnpm typecheck` before committing.
- **API Routes**: New HTTP routes must be registered in `router.go`, documented in `openapi.yaml`, and covered by the route-parity test (`TestRegisteredRoutesMatchOpenAPI`).
- **Authorization**: All cluster-scoped routes must include `withClusterContext()` and `requireClusterAccess()` middleware. Unauthorized access returns 404, not 403.
- **Testing**: New features should include unit tests. Changes to behavior should update existing tests. Aim for 50%+ coverage.

## Pull Request Workflow

1. **Before submitting**: Ensure all local checks pass (gofmt, go vet, tests, lint, typecheck)
2. **PR description**: Clearly describe the what, why, and how of your changes. Reference relevant ADRs if applicable.
3. **CI gates**: Your PR must pass all CI checks before review:
   - Backend: formatting, vet, golangci-lint, tests, coverage ≥ 50%, build
   - Frontend: lint, typecheck, test, build
   - Manifests: kustomize rendering, helm lint, docker compose config
   - Runtime: Compose-based integration tests
4. **Review**: After CI passes, the PR will be reviewed. Address feedback promptly.

## Reporting Issues

When filing an issue, please include:

- Clear description of the problem
- Steps to reproduce
- Expected behavior vs actual behavior
- Screenshots or error logs if applicable
- Environment details (OS, Go version, Node version, Kubernetes version)

## Security Disclosure

For security vulnerabilities, please follow the process outlined in [SECURITY.md](SECURITY.md). Do not file public issues for security vulnerabilities.
