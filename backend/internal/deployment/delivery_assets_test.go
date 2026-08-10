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
		"scripts/verify-fast.ps1": {
			"git", "diff", "--check", "go vet", "go test", "typecheck", "kubectl", "kustomize",
		},
		"backend/cmd/credential-reencrypt/main.go": {
			"--apply", "batch-size", "max-records", "pg_try_advisory_lock",
		},
		"backend/cmd/audit-archive/main.go": {
			"verify\", false", "trusted-public-key-file", "from-id", "max-records",
		},
		"backend/internal/audit/archive.go": {
			"Ed25519", "ErrUntrustedSigner", "MaxArchiveRecords", "refusing to overwrite",
		},
		"backend/cmd/identity-readiness/main.go": {
			"policy-file", "discovery-file", "jwks-file", "report.Ready",
		},
		"backend/internal/identityreadiness/readiness.go": {
			"PKCERequired", "admin_prelinked_subject", "offline_secret_manager", "elliptic.P256().IsOnCurve",
		},
		"backend/cmd/recovery-readiness/main.go": {
			"policy-file", "logical-restore-evidence", "ReadyForPITRHAImplementation",
		},
		"backend/internal/recoveryreadiness/readiness.go": {
			"ProductionRecoveryValidated", "RiskAcceptanceExpiresAt", "WriterFencing", "SourceDestroyedBeforeRestore",
		},
		"backend/migrations/000016_credential_reencryption_runs.up.sql": {
			"credential_reencryption_runs", "source_key_versions", "error_code",
		},
		"backend/migrations/000017_metrics_history.up.sql": {
			"metric_collection_runs", "metric_samples", "metric_samples_run_cluster_fk",
			"metric_collection_runs_result_consistency_check", "window_milliseconds",
		},
		"backend/internal/metricshistory/service.go": {
			"defaultRetention", "defaultMaxSamplesPerCollection", "defaultMaxQueryWindow",
			"ErrInvalidCollection", "SourceUnavailable", "DeleteExpired",
		},
		"backend/internal/metricshistory/collector.go": {
			"MaxConcurrentClusters", "PerClusterTimeout", "METRICS_API_TIMEOUT",
			"COLLECTION_LIMIT_REACHED", "allocateSamples", "CleanupOnce",
		},
		"backend/internal/metricshistory/quantity.go": {
			"resource.ParseQuantity", "resource.Nano", "maximumMemoryQuantity",
		},
		"backend/internal/httpserver/metrics_history.go": {
			"resource_kind", "requiredHistoryTime", "METRICS_HISTORY_QUERY_FAILED", "CLUSTER_NOT_FOUND",
		},
		"backend/internal/metricshistory/evaluator.go": {
			"insufficient_data", "firing", "normal", "ObservedSpanSeconds", "ErrInvalidEvaluation",
		},
		"backend/internal/httpserver/metrics_evaluation.go": {
			"operator", "threshold", "for_seconds", "minimum_points", "METRICS_EVALUATION_FAILED",
		},
		"frontend/src/components/MetricsHistoryPanel.vue": {
			"资源趋势", "缺失采集不会补零", "coverage.missing", "response.truncated",
		},
		".env.example": {"CREDENTIAL_DECRYPTION_KEYS={}"},
		"docs/adr/0030-controlled-application-credential-key-reencryption.md": {
			"defaults to dry-run", "FOR UPDATE SKIP LOCKED", "at most eight", "does not introduce envelope encryption",
		},
		"docs/database/credential-key-rotation.md": {
			"Safety Preconditions", "--apply", "UNKNOWN_KEY_VERSION", "Never query, export or paste",
		},
		"docs/changes/2026-07-28-credential-key-reencryption.md": {
			"M20 Phase 9", "credential-reencrypt", "v2-only backend", "30334216631",
		},
		"docs/adr/0031-offline-signed-audit-archives.md": {
			"external trust anchor", "read-only PostgreSQL", "1..10000", "does not add an",
		},
		"docs/database/audit-archive.md": {
			"Safety Preconditions", "trusted-public-key-file", "record limit exceeded", "not hash chained",
		},
		"docs/changes/2026-07-28-signed-audit-archives.md": {
			"M20 Phase 10", "audit-archive", "one-byte mutation", "30340088789",
		},
		"docs/adr/0032-offline-oidc-mfa-readiness-gate.md": {
			"provider-neutral", "PKCE S256", "automatically linked by email", "no HTTP endpoint",
		},
		"docs/security/identity-readiness.md": {
			"Required Decisions", "--network none", "14 checks", "Production Integration Boundary",
		},
		"docs/changes/2026-07-28-identity-readiness-gate.md": {
			"M20 Phase 11", "identity-readiness", "MFA/email-linking downgrades", "30345051371",
		},
		"docs/adr/0033-offline-production-recovery-readiness.md": {
			"15 controls", "180 days", "production_recovery_validated: false", "network disabled",
		},
		"docs/database/recovery-readiness.md": {
			"Required Decisions", "--network none", "15 checks", "Production Validation Boundary",
		},
		"docs/changes/2026-07-28-recovery-readiness-gate.md": {
			"M20 Phase 12", "recovery-readiness", "ready_for_pitr_ha_implementation", "30348664880",
		},
		"docs/references/krm-ratel-gap-analysis.md": {
			"Evidence Boundary", "Priority 0", "Fixed cross-cluster promotion", "Explicit Non-Goals",
		},
		"docs/changes/2026-07-28-product-roadmap-reprioritization.md": {
			"M21", "M22", "M23", "M24", "M25", "M26", "Feature-Count Parity", "30351531959",
		},
		"docs/adr/0034-bounded-postgres-metrics-history.md": {
			"seven-day retention", "1,800", "1,440", "does not insert zeroes", "generic PromQL",
		},
		"docs/changes/2026-07-28-m21-bounded-metrics-history-foundation.md": {
			"M21 Phase 1", "Migration 17", "sparse", "1,440 points", "Background collection",
		},
		"docs/adr/0035-bounded-background-metrics-collection.md": {
			"60 seconds", "four clusters", "round-robin", "METRICS_QUANTITY_INVALID", "leader-election",
		},
		"docs/changes/2026-07-28-m21-bounded-background-metrics-collector.md": {
			"M21 Phase 2", "Go 1.25.12", "1,800-point cap", "stable failure codes",
		},
		"docs/adr/0036-authenticated-exact-series-metrics-history.md": {
			"24-hour", "1,440", "Missing collections never become", "METRICS_HISTORY_QUERY_FAILED",
		},
		"docs/changes/2026-07-29-m21-authenticated-exact-series-history-api.md": {
			"M21 Phase 3", "/api/v1/clusters/{cluster_id}/metrics/history", "restart durability", "one sparse gap",
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
		"scripts/e2e-audit-archive.ps1": {
			"audit-archive", "trusted-public-key-file", "record limit", ".artifacts\\audit-archive", "finally",
		},
		"scripts/e2e-identity-readiness.ps1": {
			"--network", "none", "issuer_mismatch_rejected", "automatic_email_linking_rejected", ".artifacts\\identity-readiness",
		},
		"scripts/e2e-recovery-readiness.ps1": {
			"--network", "none", "inadequate_copies_rejected", "production_recovery_validated", ".artifacts\\recovery-readiness",
		},
		"scripts/e2e-postgres-backup-restore.ps1": {
			"aiops.logical-restore-evidence/v1", "source_destroyed_before_restore", ".artifacts\\postgres-recovery", "finally",
		},
		"scripts/e2e-metrics-history.ps1": {
			"cross_cluster_isolation", "exact_series_isolation", "restart_durability", ".artifacts\\metrics-history-e2e", "cleanup_complete",
		},
		"scripts/e2e-m21-history-kind.ps1": {
			"aiops-test", "metrics/history/evaluate", "insufficient_data", ".artifacts\\m21-history-kind", "cleanup_complete",
			"docker image inspect", "docker pull", "docker build", "--platform linux/amd64", "load", "docker-image", "imagePullPolicy: IfNotPresent", "docker image rm",
		},
		".github/workflows/ci.yml": {
			"pull_request:", "workflow_call:", "contents: read", "ubuntu-24.04",
			"runtime_required", "credential-drill:", "audit-drill:", "identity-drill:", "recovery-drill:", "CI result",
			"e2e-credential-reencryption.ps1", "e2e-audit-archive.ps1", "e2e-identity-readiness.ps1", "e2e-recovery-readiness.ps1", "e2e-metrics-history.ps1",
			"docker image save", "docker compose build frontend", "docker compose up -d --no-build", "docker compose down --volumes --remove-orphans",
		},
		".github/workflows/release.yml": {
			"v*.*.*-rc.*", "uses: ./.github/workflows/ci.yml", "SHA256SUMS",
			"release-manifest.mjs", "aiops-platform-offline-$VERSION", "--require-signatures",
			"gh release create", "--verify-tag", "--prerelease",
		},
		"scripts/release-manifest.mjs": {
			"aiops.release-manifest/v1", "release_candidate", "productionReady: false",
			"OCI archive", "SHA256SUMS", "requireSignatures", "cosign",
		},
		"scripts/release-verify.ps1": {
			"StrictSupplyChain", "StrictSignatures", "OFFLINE-SHA256SUMS",
			"release-manifest.mjs", "verified-local-key", "m97-release-rehearsal/v1",
		},
		"docs/release-candidate-operations.md": {
			"release-manifest.json", "Helm Install And Upgrade", "Kustomize Install And Upgrade",
			"Offline Package", "SHA256SUMS.bundle", "productionReady=false",
		},
		".github/workflows/real-kind-e2e.yml": {
			"schedule:", "self-hosted", "aiops-kind", "e2e-diagnosis-kind.ps1", "e2e-m21-history-kind.ps1",
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
		"deploy/helm/aiops-platform/Chart.yaml": {
			"apiVersion: v2", "name: aiops-platform", "type: application",
			"version:", "appVersion:",
		},
		"deploy/helm/aiops-platform/values.yaml": {
			"existingSecret: aiops-secrets", "namespace:", "backend:", "frontend:", "postgres:",
			"pod-security.kubernetes.io/enforce: restricted",
		},
		"deploy/helm/aiops-platform/values.schema.json": {
			`"required": ["backend", "frontend", "postgres", "existingSecret"]`,
			`"pullPolicy"`, `"enum": ["Always", "IfNotPresent", "Never"]`,
		},
		"deploy/helm/aiops-platform/templates/backend.yaml": {
			"kind: Deployment", "kind: Service", "prometheus.io/scrape",
			"runAsNonRoot: true", "readOnlyRootFilesystem: true", "drop: [ALL]",
		},
		"deploy/helm/aiops-platform/templates/network-policies.yaml": {
			"kind: NetworkPolicy", "name: default-deny", "policyTypes: [Ingress, Egress]",
		},
		"deploy/helm/aiops-platform/templates/_helpers.tpl": {
			`"aiops-platform.namespace"`, `"aiops-platform.labels"`, `"aiops-platform.selectorLabels"`,
		},
		"SECURITY.md": {
			"Supported Versions", "Reporting a Vulnerability",
			"GitHub Security Advisory", "Disclosure timeline",
			"Authorization returns 404", "Restricted pod security",
		},
		"CHANGELOG.md": {
			"Keep a Changelog", "Semantic Versioning",
			"baseline-m35-20260731", "baseline-m34-20260731", "baseline-m33-20260731",
		},
		"docs/security/license-allowlist.json": {
			`"allowedLicenses"`, `"reviewRequiredLicenses"`,
			`"MIT"`, `"ISC"`, `"BSD-2-Clause"`, `"BSD-3-Clause"`, `"Apache-2.0"`,
			`"GPL"`, `"LGPL"`, `"UNKNOWN"`, `"SEE-LICENSE"`,
		},
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

func TestVerificationNativeWrappersJudgeCommandsByExitCode(t *testing.T) {
	root := repositoryRoot(t)
	for _, relativePath := range []string{"scripts/verify.ps1", "scripts/verify-fast.ps1"} {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		text := string(contents)
		for _, marker := range []string{
			"$previousErrorAction = $ErrorActionPreference",
			"$ErrorActionPreference = 'Continue'",
			"$exitCode = $LASTEXITCODE",
			"$ErrorActionPreference = $previousErrorAction",
		} {
			if !strings.Contains(text, marker) {
				t.Errorf("%s must handle native stderr and judge success by exit code; missing %q", relativePath, marker)
			}
		}
	}
}

func TestNewKindAcceptanceScriptsRemainWindowsPowerShellCompatible(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range []string{
		"e2e-m21-history-kind.ps1",
		"e2e-m23-release-lifecycle-kind.ps1",
		"e2e-m24-cross-cluster-promotion-kind.ps1",
		"e2e-m25-workload-protection-kind.ps1",
	} {
		contents, err := os.ReadFile(filepath.Join(root, "scripts", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(contents), "??") {
			t.Errorf("%s uses the PowerShell 7-only null-coalescing operator", name)
		}
	}
}

func TestM25KindAcceptanceWaitsForVeleroCRDBeforeApplyingBackups(t *testing.T) {
	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, "scripts", "e2e-m25-workload-protection-kind.ps1"))
	if err != nil {
		t.Fatalf("read M25 kind acceptance script: %v", err)
	}
	text := string(contents)

	waitMarker := "@('wait', '--for=condition=Established', 'crd/backups.velero.io', '--timeout=60s')"
	fixtureMarker := "deploy\\m25-workload-protection-e2e\\primary\\sample-backups.yaml"
	waitIndex := strings.Index(text, waitMarker)
	fixtureIndex := strings.Index(text, fixtureMarker)
	if waitIndex < 0 {
		t.Fatalf("M25 acceptance must wait for backups.velero.io CRD establishment")
	}
	if fixtureIndex < 0 {
		t.Fatalf("M25 acceptance must apply sample backups after installing the CRD")
	}
	if waitIndex > fixtureIndex {
		t.Fatalf("M25 acceptance must wait for CRD establishment before applying sample backups")
	}

	kustomization, err := os.ReadFile(filepath.Join(root, "deploy", "m25-workload-protection-e2e", "primary", "kustomization.yaml"))
	if err != nil {
		t.Fatalf("read M25 primary kustomization: %v", err)
	}
	if strings.Contains(string(kustomization), "sample-backups.yaml") {
		t.Fatalf("sample backups must not share the CRD installation apply operation")
	}
}

func TestM21KindAcceptanceScriptUsesNativeWrapperForDockerBuild(t *testing.T) {
	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, "scripts", "e2e-m21-history-kind.ps1"))
	if err != nil {
		t.Fatalf("read M21 kind acceptance script: %v", err)
	}
	text := string(contents)

	if !strings.Contains(text, "[string]$InputText") {
		t.Fatalf("Invoke-NativeText must accept stdin text so Docker BuildKit stderr does not bypass native command handling")
	}
	if !strings.Contains(text, "System.Management.Automation.ErrorRecord") || !strings.Contains(text, "$_.Exception.Message") {
		t.Fatalf("Invoke-NativeText must preserve native stderr text instead of logging PowerShell ErrorRecord type names")
	}
	if strings.Contains(text, "| & docker build") {
		t.Fatalf("docker build must be invoked through Invoke-NativeText instead of a direct PowerShell native pipeline")
	}
	if !strings.Contains(text, "-FilePath 'docker' -Arguments @('build'") || !strings.Contains(text, "-InputText $workloadDockerfile") {
		t.Fatalf("docker build must route the generated Dockerfile through Invoke-NativeText -InputText")
	}

	buildIndex := strings.Index(text, "docker build --platform linux/amd64")
	if buildIndex < 0 {
		t.Fatalf("script is missing the M21 disposable docker build step")
	}
	initializationIndex := strings.Index(text, "$workloadImageBuilt = $false")
	if initializationIndex < 0 {
		t.Fatalf("script must initialize the disposable image cleanup flag")
	}
	if initializationIndex > buildIndex {
		t.Fatalf("$workloadImageBuilt must be initialized before the docker build step so cleanup state is not reset after a successful build")
	}
	if strings.Contains(text[buildIndex:], "$workloadImageBuilt = $false") {
		t.Fatalf("$workloadImageBuilt must not be reset after docker build succeeds")
	}
}
