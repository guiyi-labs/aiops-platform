package deployment_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliveryAssetsCoverVerificationAndThesisMaterials(t *testing.T) {
	root := repositoryRoot(t)
	required := map[string][]string{
		"scripts/verify.ps1": {
			"go test", "pnpm typecheck", "pnpm test", "pnpm build",
			"credential-reencrypt", "docker compose config", "kubectl kustomize", "/api/v1/health/ready",
		},
		"backend/cmd/credential-reencrypt/main.go": {
			"--apply", "batch-size", "max-records", "pg_try_advisory_lock",
		},
		"backend/migrations/000016_credential_reencryption_runs.up.sql": {
			"credential_reencryption_runs", "source_key_versions", "error_code",
		},
		".env.example": {"CREDENTIAL_DECRYPTION_KEYS={}"},
		"docs/adr/0030-controlled-application-credential-key-reencryption.md": {
			"defaults to dry-run", "FOR UPDATE SKIP LOCKED", "at most eight", "does not introduce envelope encryption",
		},
		"docs/database/credential-key-rotation.md": {
			"Safety Preconditions", "--apply", "UNKNOWN_KEY_VERSION", "Never query, export or paste",
		},
		"docs/changes/2026-07-28-credential-key-reencryption.md": {
			"M20 Phase 9", "credential-reencrypt", "v2-only backend", "hosted CI pending",
		},
		"scripts/e2e-kind.ps1": {
			"kubectl create token", "/api/v1/clusters", "/diagnoses",
			"remediations/preview", "operations/preview", "deployment.scale",
			"cronjob.resume", "cronjob.suspend", "restored_replicas",
			"Idempotency-Key", "m17Fixtures", "finally",
		},
		"scripts/e2e-diagnosis-kind.ps1": {
			"kind-v0.30.0.exe", "synthetic-not-ready", "stalled-deployment",
			"node.not_ready.v1", "deployment.replicas_unavailable.v1", "finally",
		},
		"scripts/e2e-metrics-kind.ps1": {
			"metrics-server", "ManifestSHA256", "RequireMetrics", "Wait-MetricsSamples", "finally",
		},
		"scripts/e2e-fleet-kind.ps1": {
			"kind-v0.30.0.exe", "fleet/health?limit=20", "timed_out", "unavailable",
			"preexisting_kind_clusters_preserved", ".artifacts\\fleet-e2e", "finally",
		},
		"scripts/e2e-global-search-kind.ps1": {
			"kind-v0.30.0.exe", "fleet/resources/search", "cluster_limit=1", "TIMEOUT", "QUERY_FAILED",
			"preexisting_kind_clusters_preserved", ".artifacts\\search-e2e", "finally",
		},
		"scripts/e2e-credential-reencryption.ps1": {
			"credential-reencrypt", "insecure-skip-tls-verify", "REENCRYPTION_FAILED",
			"v2_only_backend_decryption", ".artifacts\\credential-reencryption", "finally",
		},
		".github/workflows/ci.yml": {
			"pull_request:", "workflow_call:", "contents: read", "ubuntu-24.04",
			"e2e-credential-reencryption.ps1", "docker compose up -d --build", "docker compose down --volumes --remove-orphans",
		},
		".github/workflows/release.yml": {
			"v*.*.*", "uses: ./.github/workflows/ci.yml", "SHA256SUMS",
			"gh release create", "--verify-tag",
		},
		".github/workflows/real-kind-e2e.yml": {
			"schedule:", "self-hosted", "aiops-kind", "e2e-diagnosis-kind.ps1",
			"e2e-fleet-kind.ps1", "e2e-global-search-kind.ps1", "retention-days: 14",
		},
		".github/dependabot.yml": {"github-actions", "gomod", "npm", "weekly"},
		"docs/ci-release.md": {
			"Required Branch Protection", "package rehearsal", "SHA256SUMS",
			"aiops-kind", "actionlint", "does not publish a container registry tag",
		},
		"deploy/metrics-server-kind/README.md": {
			"v0.8.0", "SHA-256", "Apache License 2.0", "kubelet-insecure-tls",
		},
		"deploy/demo-scenarios/m17-resources.yaml": {
			"StatefulSet", "DaemonSet", "ReplicaSet", "CronJob",
			"HorizontalPodAutoscaler", "ResourceQuota", "LimitRange", "Secret",
		},
		"deploy/demo-scenarios/m18-diagnosis-resources.yaml": {
			"m18-pending-pvc", "m18-broken-ingress", "m18-saturated-hpa",
		},
		"backend/internal/diagnosis/testdata/m18-fixtures.json": {
			"ready-memory-pressure", "pending-with-warning", "limited-at-maximum", "backend-without-ready-addresses",
		},
		"docs/changes/2026-07-27-evidence-based-diagnosis-expansion.md": {
			"M18", "node.pressure.v1", "persistentvolumeclaim.pending.v1", "sustained restart",
		},
		"docs/changes/2026-07-27-controlled-operations-catalog.md": {
			"M19", "000014_controlled_operations_catalog", "deployment.scale",
			"cronjob.suspend", ".artifacts/e2e-kind/e2e-kind-20260727-180557.json",
		},
		"docs/adr/0024-resource-originated-controlled-operations.md": {
			"deployment.rollout_restart", "deployment.scale", "cronjob.resume",
			"dryRun=All", "Deployment rollback is deferred",
		},
		"docs/changes/2026-07-27-bounded-multi-cluster-health.md": {
			"M20 Phase 1", "/api/v1/fleet/health", "four concurrent cluster workers",
			"partial", "two-cluster", "verify-20260727-190133.json", "390x844",
		},
		"docs/changes/2026-07-27-two-cluster-fleet-e2e.md": {
			"M20 Phase 2", "physically distinct", "timed_out", "unavailable",
			"fleet-e2e-20260727-193711.json", "verify-20260727-194724.json",
			"all eight cleanup assertions",
		},
		"docs/adr/0025-bounded-multi-cluster-health-fanout.md": {
			"at most 20 enabled clusters", "four clusters", "four-second",
			"100 sampled", "HTTP 200",
		},
		"docs/adr/0026-bounded-global-resource-search.md": {
			"at most 20 enabled clusters", "four cluster workers", "four-second budget",
			"pods", "deployments", "services", "ingresses",
		},
		"docs/changes/2026-07-27-bounded-global-resource-search.md": {
			"M20 Phase 3", "/api/v1/fleet/resources/search", "clusters_remaining",
			"TIMEOUT", "QUERY_FAILED", "390x844",
		},
		"docs/adr/0027-user-owned-global-search-filters.md": {
			"at most 20 filters", "case-insensitively", "advisory transaction lock",
			"SCHEMA_VERSION", "QUERY_SHAPE", "audit",
		},
		"docs/changes/2026-07-27-user-owned-global-search-filters.md": {
			"M20 Phase 4", "/api/v1/fleet/resources/search/filters", "000015_saved_global_search_filters",
			"22 concurrent creates", "20 HTTP 201", "390x844",
		},
		"docs/changes/2026-07-27-two-cluster-global-search-e2e.md": {
			"M20 Phase 5", "physically distinct", "search-e2e-20260727-225358.json",
			"TIMEOUT", "QUERY_FAILED", "all eight cleanup assertions",
		},
		"docs/adr/0028-versioned-ci-release-pipeline.md": {
			"pull_request_target", "package-only", "SHA256SUMS", "aiops-kind",
			"does not push to a", "Creating that commit and release tag remains a human action.",
		},
		"docs/changes/2026-07-28-versioned-ci-release-pipeline.md": {
			"M20 Phase 6", "actionlint 1.7.7", "contents: read", "--verify-tag",
			"Dependabot", "human-approved initial commit",
		},
		"docs/changes/2026-07-27-common-workload-policy-coverage.md": {
			"M17", "Secret", "Kubernetes RBAC", ".artifacts/verification/verify-",
		},
		"scripts/demo-up.ps1": {
			"e2e-kind.ps1", "KeepPlatformCluster", "demo-ready",
		},
		"scripts/demo-down.ps1": {
			"/api/v1/clusters", "demo-kind-", "CleanupDemoResources",
		},
		"scripts/capture-thesis-screenshots.ps1": {
			"capture-thesis-screenshots.mjs", "AIOPS_ADMIN_PASSWORD", "docs\\thesis\\screenshots",
		},
		"scripts/capture-thesis-screenshots.mjs": {
			"Page.captureScreenshot", "Runtime.evaluate", "msedge.exe",
		},
		"docs/thesis/system-diagrams.md": {
			"```mermaid", "flowchart", "erDiagram", "sequenceDiagram",
		},
		"docs/thesis/test-matrix.md":         {"Backend", "Frontend", "Real kind"},
		"docs/thesis/environment.md":         {"Docker Desktop", "Kubernetes", "PostgreSQL"},
		"docs/thesis/defense-demo-script.md": {"10", "ImagePullBackOff", "CrashLoopBackOff"},
		"docs/thesis/dependency-licenses.md": {"Go production dependencies", "Frontend production dependencies"},
		"docs/thesis/references.md":          {"KubeSphere", "KRM", "Ratel"},
		"docs/thesis/demo-environment.md":    {"Preparation", "Cleanup", "short-lived credential"},
		"docs/thesis/screenshots/README.md":  {"Dashboard", "Clusters", "Diagnoses"},
	}

	for name, markers := range required {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("required delivery asset %s is missing: %v", name, err)
			continue
		}
		text := string(contents)
		for _, marker := range markers {
			if !strings.Contains(text, marker) {
				t.Errorf("delivery asset %s is missing marker %q", name, marker)
			}
		}
	}
}
