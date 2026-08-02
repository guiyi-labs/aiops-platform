package imagepolicy

import (
	"testing"
	"time"
)

var testAt = time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)

// usage builds one ImageUsage from a raw image reference, so tests read like
// the cluster data they stand for.
func usage(ref, namespace, kind, workload, container, pullPolicy string) ImageUsage {
	img := ParseImage(ref)
	img.PullPolicy = pullPolicy
	return ImageUsage{
		Image: img,
		Container: ContainerRef{
			Namespace:    namespace,
			WorkloadKind: kind,
			WorkloadName: workload,
			Container:    container,
		},
	}
}

// findingsByCode indexes findings so assertions do not depend on slice order.
func findingsByCode(s Status) map[string][]Finding {
	out := map[string][]Finding{}
	for _, f := range s.Findings {
		out[f.Code] = append(out[f.Code], f)
	}
	return out
}

// TestEvaluate_EmptyBundleIsSkipped guards the "no data is not a pass" rule:
// an empty observation bundle must not report a clean cluster.
func TestEvaluate_EmptyBundleIsSkipped(t *testing.T) {
	in := Inputs{}
	if !in.Empty() {
		t.Fatal("Empty() = false for a zero bundle")
	}

	got := Evaluate(7, in, testAt)
	if got.Total != 0 || got.Failed != 0 || got.Passed != 0 {
		t.Fatalf("counters = %d/%d/%d, want all zero", got.Total, got.Failed, got.Passed)
	}
	if got.ImagesTotal != 0 || got.ContainersTotal != 0 {
		t.Fatalf("inventory = %d images / %d containers, want zero", got.ImagesTotal, got.ContainersTotal)
	}
	// JSON-facing collections must never be nil, or the API emits null.
	if got.Findings == nil || got.BySeverity == nil || got.ByFamily == nil {
		t.Fatalf("collections must be non-nil for JSON: %+v", got)
	}
	if got.ClusterID != 7 || !got.EvaluatedAt.Equal(testAt) {
		t.Fatalf("identity = cluster %d at %v", got.ClusterID, got.EvaluatedAt)
	}
}

// TestEvaluate_MutableTagIsWarning covers the primary reproducibility risk:
// :latest (and an omitted tag, which Kubernetes treats as :latest) means a
// redeploy can silently change what runs.
func TestEvaluate_MutableTagIsWarning(t *testing.T) {
	cases := []struct {
		name string
		ref  string
	}{
		{"explicit latest", "registry.io/team/api:latest"},
		{"omitted tag", "registry.io/team/api"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(7, Inputs{Usages: []ImageUsage{
				usage(tc.ref, "shop", "Deployment", "api", "app", ""),
			}}, testAt)

			byCode := findingsByCode(got)
			if len(byCode[CodeMutableTag]) != 1 {
				t.Fatalf("want one %s finding, got %+v", CodeMutableTag, got.Findings)
			}
			f := byCode[CodeMutableTag][0]
			if f.Severity != SeverityWarning {
				t.Fatalf("severity = %q, want %q", f.Severity, SeverityWarning)
			}
			if f.Details["family"] != FamilySupplyChain {
				t.Fatalf("family = %q, want %q", f.Details["family"], FamilySupplyChain)
			}
			// An omitted tag must be reported as the effective ":latest".
			if f.Details["tag"] != "latest" {
				t.Fatalf("tag detail = %q, want latest", f.Details["tag"])
			}
			if f.Details["remediation"] == "" || f.Details["rationale"] == "" {
				t.Fatalf("finding must explain itself: %+v", f.Details)
			}
			if got.MutableTagImages != 1 {
				t.Fatalf("MutableTagImages = %d, want 1", got.MutableTagImages)
			}
			// A mutable tag is already the stronger finding, so the weaker
			// "no digest pin" must not also fire for the same image.
			if len(byCode[CodeNoDigestPin]) != 0 {
				t.Fatalf("mutable tag must not also report %s", CodeNoDigestPin)
			}
			if got.Failed != len(got.Findings) {
				t.Fatalf("Failed = %d but %d findings", got.Failed, len(got.Findings))
			}
			if got.Passed != got.Total-got.Failed {
				t.Fatalf("Passed = %d, want Total-Failed = %d", got.Passed, got.Total-got.Failed)
			}
		})
	}
}

// TestEvaluate_FixedTagWithoutDigestIsInfo covers the softer case: the tag is
// pinned but the registry could still re-point it.
func TestEvaluate_FixedTagWithoutDigestIsInfo(t *testing.T) {
	got := Evaluate(7, Inputs{Usages: []ImageUsage{
		usage("registry.io/team/api:v1.4.2", "shop", "Deployment", "api", "app", "IfNotPresent"),
	}}, testAt)

	byCode := findingsByCode(got)
	if len(byCode[CodeNoDigestPin]) != 1 {
		t.Fatalf("want one %s finding, got %+v", CodeNoDigestPin, got.Findings)
	}
	if sev := byCode[CodeNoDigestPin][0].Severity; sev != SeverityInfo {
		t.Fatalf("severity = %q, want %q", sev, SeverityInfo)
	}
	if got.UnpinnedImages != 1 || got.MutableTagImages != 0 {
		t.Fatalf("counters = %d unpinned / %d mutable, want 1/0", got.UnpinnedImages, got.MutableTagImages)
	}
}

// TestEvaluate_DigestPinnedImageIsClean is the regression guard for the most
// reproducible reference of all: a digest pin must produce no finding, even
// when the tag is absent or literally ":latest".
func TestEvaluate_DigestPinnedImageIsClean(t *testing.T) {
	const digest = "sha256:6b3f1e9d0c5a4b2e8f7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a392817"
	cases := []struct {
		name string
		ref  string
	}{
		{"digest only", "registry.io/team/api@" + digest},
		{"latest plus digest", "registry.io/team/api:latest@" + digest},
		{"tag plus digest", "registry.io/team/api:v1.4.2@" + digest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(7, Inputs{Usages: []ImageUsage{
				usage(tc.ref, "shop", "Deployment", "api", "app", "IfNotPresent"),
			}}, testAt)

			if len(got.Findings) != 0 {
				t.Fatalf("digest-pinned image must be clean, got %+v", got.Findings)
			}
			if got.Failed != 0 || got.Passed != got.Total {
				t.Fatalf("counters = %d failed / %d passed of %d", got.Failed, got.Passed, got.Total)
			}
			if got.MutableTagImages != 0 || got.UnpinnedImages != 0 {
				t.Fatalf("pinned image counted as risky: %+v", got)
			}
		})
	}
}

// TestEvaluate_PullAlwaysWithMutableTag covers the compounding case where
// every restart re-pulls whatever :latest currently points at.
func TestEvaluate_PullAlwaysWithMutableTag(t *testing.T) {
	got := Evaluate(7, Inputs{Usages: []ImageUsage{
		usage("registry.io/team/api:latest", "shop", "Deployment", "api", "app", "Always"),
	}}, testAt)

	byCode := findingsByCode(got)
	if len(byCode[CodePullAlwaysLatest]) != 1 {
		t.Fatalf("want one %s finding, got %+v", CodePullAlwaysLatest, got.Findings)
	}
	if pp := byCode[CodePullAlwaysLatest][0].Details["pull_policy"]; pp != "Always" {
		t.Fatalf("pull_policy detail = %q, want Always", pp)
	}

	// Always with a pinned tag is a normal, safe configuration.
	clean := Evaluate(7, Inputs{Usages: []ImageUsage{
		usage("registry.io/team/api:v1.4.2", "shop", "Deployment", "api", "app", "Always"),
	}}, testAt)
	if len(findingsByCode(clean)[CodePullAlwaysLatest]) != 0 {
		t.Fatalf("Always with a fixed tag must not fire %s", CodePullAlwaysLatest)
	}
}

// TestEvaluate_SharedAcrossNamespaces checks the blast-radius finding and, in
// the same run, that identical references from several containers collapse
// into one image entry.
func TestEvaluate_SharedAcrossNamespaces(t *testing.T) {
	got := Evaluate(7, Inputs{Usages: []ImageUsage{
		usage("registry.io/team/api:v1.4.2", "shop", "Deployment", "api", "app", ""),
		usage("registry.io/team/api:v1.4.2", "billing", "Deployment", "api", "app", ""),
		usage("registry.io/team/api:v1.4.2", "billing", "StatefulSet", "worker", "app", ""),
	}}, testAt)

	if got.ImagesTotal != 1 {
		t.Fatalf("ImagesTotal = %d, want 1 (same reference deduplicated)", got.ImagesTotal)
	}
	if got.ContainersTotal != 3 {
		t.Fatalf("ContainersTotal = %d, want 3", got.ContainersTotal)
	}

	byCode := findingsByCode(got)
	if len(byCode[CodeSharedAcrossNamespaces]) != 1 {
		t.Fatalf("want one %s finding, got %+v", CodeSharedAcrossNamespaces, got.Findings)
	}
	f := byCode[CodeSharedAcrossNamespaces][0]
	if f.Details["namespaces"] != "2" {
		t.Fatalf("namespaces detail = %q, want 2", f.Details["namespaces"])
	}
	if f.Details["containers"] != "3" {
		t.Fatalf("containers detail = %q, want 3", f.Details["containers"])
	}

	// A single-namespace image must not fire the finding.
	single := Evaluate(7, Inputs{Usages: []ImageUsage{
		usage("registry.io/team/api:v1.4.2", "shop", "Deployment", "api", "app", ""),
	}}, testAt)
	if len(findingsByCode(single)[CodeSharedAcrossNamespaces]) != 0 {
		t.Fatalf("single-namespace image must not fire %s", CodeSharedAcrossNamespaces)
	}
}

// TestEvaluate_RepositoryTagSkew covers version drift: one repository pulled
// under several tags.
func TestEvaluate_RepositoryTagSkew(t *testing.T) {
	got := Evaluate(7, Inputs{Usages: []ImageUsage{
		usage("registry.io/team/api:v1.4.2", "shop", "Deployment", "api", "app", ""),
		usage("registry.io/team/api:v1.3.0", "shop", "Deployment", "api-canary", "app", ""),
	}}, testAt)

	byCode := findingsByCode(got)
	if len(byCode[CodeMultipleTags]) != 1 {
		t.Fatalf("want one %s finding, got %+v", CodeMultipleTags, got.Findings)
	}
	// Tags are listed in a stable, sorted order so the console output does not
	// churn between runs.
	if tags := byCode[CodeMultipleTags][0].Details["tags"]; tags != "v1.3.0,v1.4.2" {
		t.Fatalf("tags detail = %q, want v1.3.0,v1.4.2", tags)
	}
	if got.ImagesTotal != 2 {
		t.Fatalf("ImagesTotal = %d, want 2", got.ImagesTotal)
	}
}

// TestEvaluate_FindingsAreSortedBySeverity keeps the console's most urgent row
// at the top and makes the output deterministic.
func TestEvaluate_FindingsAreSortedBySeverity(t *testing.T) {
	got := Evaluate(7, Inputs{Usages: []ImageUsage{
		usage("registry.io/team/zeta:v1.0.0", "shop", "Deployment", "zeta", "app", ""),
		usage("registry.io/team/alpha:latest", "shop", "Deployment", "alpha", "app", ""),
	}}, testAt)

	if len(got.Findings) < 2 {
		t.Fatalf("expected several findings, got %+v", got.Findings)
	}
	if got.Findings[0].Severity != SeverityWarning {
		t.Fatalf("first finding severity = %q, want the warning first", got.Findings[0].Severity)
	}
	ranks := make([]int, 0, len(got.Findings))
	for _, f := range got.Findings {
		ranks = append(ranks, severityRank[f.Severity])
	}
	for i := 1; i < len(ranks); i++ {
		if ranks[i-1] > ranks[i] {
			t.Fatalf("findings not ordered by severity: %v", ranks)
		}
	}

	// Rollups must agree with the finding list.
	total := 0
	for _, n := range got.BySeverity {
		total += n
	}
	if total != len(got.Findings) {
		t.Fatalf("BySeverity sums to %d but there are %d findings", total, len(got.Findings))
	}
	if got.ByFamily[FamilySupplyChain] != len(got.Findings) {
		t.Fatalf("ByFamily[%s] = %d, want %d", FamilySupplyChain, got.ByFamily[FamilySupplyChain], len(got.Findings))
	}
}

// TestParseImage covers reference decomposition, including the registry-port
// trap where "registry.io:5000/team/api" must not be read as tag "5000/...".
func TestParseImage(t *testing.T) {
	const digest = "sha256:6b3f1e9d0c5a4b2e8f7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a392817"
	cases := []struct {
		name string
		ref  string
		want ImageInfo
	}{
		{"bare name", "nginx", ImageInfo{Repository: "nginx"}},
		{"name and tag", "nginx:1.27", ImageInfo{Repository: "nginx", Tag: "1.27"}},
		{"registry path and tag", "registry.io/team/api:v1.4.2", ImageInfo{Repository: "registry.io/team/api", Tag: "v1.4.2"}},
		{"registry port no tag", "registry.io:5000/team/api", ImageInfo{Repository: "registry.io:5000/team/api"}},
		{"registry port and tag", "registry.io:5000/team/api:v1", ImageInfo{Repository: "registry.io:5000/team/api", Tag: "v1"}},
		{"digest only", "registry.io/team/api@" + digest, ImageInfo{Repository: "registry.io/team/api", Digest: digest}},
		{"tag and digest", "registry.io/team/api:v1@" + digest, ImageInfo{Repository: "registry.io/team/api", Tag: "v1", Digest: digest}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseImage(tc.ref)
			if got != tc.want {
				t.Fatalf("ParseImage(%q) = %+v, want %+v", tc.ref, got, tc.want)
			}
		})
	}
}

// TestImageRef checks the human-readable rendering used in finding summaries,
// where an omitted tag is shown as its effective ":latest".
func TestImageRef(t *testing.T) {
	const digest = "sha256:abc"
	cases := []struct {
		img  ImageInfo
		want string
	}{
		{ImageInfo{Repository: "nginx"}, "nginx:latest"},
		{ImageInfo{Repository: "nginx", Tag: "1.27"}, "nginx:1.27"},
		{ImageInfo{Repository: "nginx", Digest: digest}, "nginx:latest@" + digest},
		{ImageInfo{Repository: "nginx", Tag: "1.27", Digest: digest}, "nginx:1.27@" + digest},
	}
	for _, tc := range cases {
		if got := imageRef(tc.img); got != tc.want {
			t.Fatalf("imageRef(%+v) = %q, want %q", tc.img, got, tc.want)
		}
	}
}
