package deployment_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCIWorkflowContractsAreParseableAndBounded(t *testing.T) {
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
				"actions/checkout@11d5960a326750d5838078e36cf38b85af677262",
				"actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff",
				"actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020",
				"actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
				"go test -p=1 -count=1 ./...", "pnpm install --frozen-lockfile",
				"docker compose up -d --build", "docker compose down --volumes --remove-orphans",
			},
			forbidden: []string{"pull_request_target", "secrets.", "contents: write"},
		},
		{
			name: ".github/workflows/release.yml",
			required: []string{
				"tags:", "workflow_dispatch:", "uses: ./.github/workflows/ci.yml",
				"SHA256SUMS", "--verify-tag", "gh release create", "contents: write",
			},
			forbidden: []string{"pull_request_target", "secrets.", "docker push"},
		},
		{
			name: ".github/workflows/real-kind-e2e.yml",
			required: []string{
				"schedule:", "workflow_dispatch:", "self-hosted", "aiops-kind",
				"e2e-diagnosis-kind.ps1", "e2e-fleet-kind.ps1", "e2e-global-search-kind.ps1",
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
