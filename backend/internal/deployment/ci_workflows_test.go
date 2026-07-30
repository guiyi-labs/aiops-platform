package deployment_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCIWorkflowContractsAreParseableAndBounded(t *testing.T) {
	const (
		checkoutAction       = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"
		setupGoAction        = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"
		setupNodeAction      = "actions/setup-node@820762786026740c76f36085b0efc47a31fe5020"
		setupPnpmAction      = "pnpm/action-setup@0ebf47130e4866e96fce0953f49152a61190b271"
		uploadArtifactAction = "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a"
	)

	root := repositoryRoot(t)
	tests := []struct {
		name      string
		required  []string
		forbidden []string
	}{
		{
			name: ".github/workflows/ci.yml",
			required: []string{
				"pull_request:", "workflow_call:", "contents: read", "ubuntu-24.04",
				checkoutAction, setupGoAction, setupNodeAction, setupPnpmAction, uploadArtifactAction,
				"go test -p=1 -count=1 ./...", "pnpm install --frozen-lockfile",
				"Change scope", "runtime_required", "credential-drill:", "audit-drill:",
				"identity-drill:", "recovery-drill:", "CI result", "Detect documentation-only changes",
				"e2e-postgres-backup-restore.ps1", "e2e-audit-archive.ps1", "e2e-identity-readiness.ps1", "e2e-recovery-readiness.ps1", "docker compose up -d --build",
				".artifacts/postgres-recovery/", ".artifacts/audit-archive/", ".artifacts/identity-readiness/", ".artifacts/recovery-readiness/", "docker compose down --volumes --remove-orphans",
			},
			forbidden: []string{"pull_request_target", "secrets.", "contents: write"},
		},
		{
			name: ".github/workflows/release.yml",
			required: []string{
				"tags:", "workflow_dispatch:", "uses: ./.github/workflows/ci.yml",
				checkoutAction, uploadArtifactAction,
				"SHA256SUMS", "--verify-tag", "gh release create", "contents: write",
			},
			forbidden: []string{"pull_request_target", "secrets.", "docker push"},
		},
		{
			name: ".github/workflows/real-kind-e2e.yml",
			required: []string{
				"schedule:", "workflow_dispatch:", "self-hosted", "aiops-kind",
				checkoutAction, uploadArtifactAction,
				"e2e-diagnosis-kind.ps1", "e2e-fleet-kind.ps1", "e2e-global-search-kind.ps1", "e2e-m21-history-kind.ps1",
				"cancel-in-progress: false", "retention-days: 14",
			},
			forbidden: []string{"pull_request_target", "secrets.", "KeepPlatformCluster"},
		},
		{
			name:     ".github/dependabot.yml",
			required: []string{"github-actions", "gomod", "npm", "weekly"},
		},
		{
			name:     ".github/actionlint.yaml",
			required: []string{"self-hosted-runner", "aiops-kind"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tt.name)))
			if err != nil {
				t.Fatal(err)
			}
			var document yaml.Node
			if err := yaml.Unmarshal(contents, &document); err != nil {
				t.Fatalf("invalid YAML: %v", err)
			}
			if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
				t.Fatalf("expected one YAML mapping document, got %#v", document)
			}
			text := string(contents)
			for _, marker := range tt.required {
				if !strings.Contains(text, marker) {
					t.Errorf("missing required marker %q", marker)
				}
			}
			for _, marker := range tt.forbidden {
				if strings.Contains(text, marker) {
					t.Errorf("contains forbidden marker %q", marker)
				}
			}
		})
	}
}

func TestDependabotGroupsKeepMajorUpdatesSeparate(t *testing.T) {
	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}

	var config struct {
		Updates []struct {
			PackageEcosystem string `yaml:"package-ecosystem"`
			Groups           map[string]struct {
				UpdateTypes []string `yaml:"update-types"`
			} `yaml:"groups"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal(contents, &config); err != nil {
		t.Fatalf("invalid Dependabot YAML: %v", err)
	}

	expectedGroups := map[string]string{
		"github-actions": "actions",
		"gomod":          "go-modules",
		"npm":            "frontend-packages",
	}
	seen := make(map[string]bool, len(expectedGroups))
	for _, update := range config.Updates {
		groupName, ok := expectedGroups[update.PackageEcosystem]
		if !ok {
			continue
		}
		group, ok := update.Groups[groupName]
		if !ok {
			t.Errorf("%s is missing group %q", update.PackageEcosystem, groupName)
			continue
		}
		if len(group.UpdateTypes) != 2 || group.UpdateTypes[0] != "minor" || group.UpdateTypes[1] != "patch" {
			t.Errorf("%s group %q update types = %v, want [minor patch]", update.PackageEcosystem, groupName, group.UpdateTypes)
		}
		seen[update.PackageEcosystem] = true
	}
	for ecosystem := range expectedGroups {
		if !seen[ecosystem] {
			t.Errorf("missing reviewed Dependabot policy for %s", ecosystem)
		}
	}
}
