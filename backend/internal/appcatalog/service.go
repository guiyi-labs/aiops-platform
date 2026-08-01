package appcatalog

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"k8s-aiops.local/backend/internal/kubernetes"
)

var (
	kubernetesNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$`)
	helmRepoNamePattern   = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$`)
)

// KubernetesSource is the subset of kubernetes.Service the deploy flow needs.
type KubernetesSource interface {
	NamespaceExists(ctx context.Context, clusterID int64, namespace string) (bool, error)
	ResourceExists(ctx context.Context, clusterID int64, path string) (bool, error)
	CreateResource(ctx context.Context, clusterID int64, path string, body []byte, dryRun bool) ([]byte, error)
}

// ChartIndexSource fetches a Helm repository's index.yaml. It is abstracted so
// tests can inject a fake without real HTTP. The credentials (basic-auth) are
// passed from the stored Repository row.
type ChartIndexSource interface {
	FetchIndex(ctx context.Context, repoURL, username, password string) ([]byte, error)
}

// HTTPIndexSource is the default ChartIndexSource implementation: fetches
// index.yaml over HTTP with optional basic-auth. It imposes a 10 MiB body
// limit and a 15-second timeout.
type HTTPIndexSource struct {
	client *http.Client
}

func NewHTTPIndexSource() *HTTPIndexSource {
	return &HTTPIndexSource{client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *HTTPIndexSource) FetchIndex(ctx context.Context, repoURL, username, password string) ([]byte, error) {
	indexURL := strings.TrimSuffix(repoURL, "/") + "/index.yaml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRepoURLInvalid, err)
	}
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRepoUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrRepoUnreachable, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRepoUnreachable, err)
	}
	return body, nil
}

type Service struct {
	kubernetes KubernetesSource
	repository DataStore
	index      ChartIndexSource
	planTTL    time.Duration
	claimTTL   time.Duration
	now        func() time.Time
}

func NewService(kubernetes KubernetesSource, repository DataStore) *Service {
	return &Service{
		kubernetes: kubernetes,
		repository: repository,
		index:      NewHTTPIndexSource(),
		planTTL:    15 * time.Minute,
		claimTTL:   2 * time.Minute,
		now:        time.Now,
	}
}

// NewTestService creates a Service with a custom ChartIndexSource for testing.
// This is used by handler tests in the httpserver package to inject a fake
// index source without real HTTP calls.
func NewTestService(kubernetes KubernetesSource, repository DataStore, index ChartIndexSource) *Service {
	return &Service{
		kubernetes: kubernetes,
		repository: repository,
		index:      index,
		planTTL:    15 * time.Minute,
		claimTTL:   2 * time.Minute,
		now:        func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) },
	}
}

// ---------------------------------------------------------------------------
// Helm repository CRUD
// ---------------------------------------------------------------------------

// CreateRepository registers a new Helm repository. The credentials (username
// / password) are stored as JSONB and never returned in API responses.
func (s *Service) CreateRepository(ctx context.Context, request CreateRepositoryRequest, actor ActorRef) (Repository, error) {
	if err := validateRepoRequest(request); err != nil {
		return Repository{}, err
	}
	if actor.ID < 1 || strings.TrimSpace(actor.Name) == "" {
		return Repository{}, ErrInvalidRequest
	}
	creds, _ := json.Marshal(map[string]string{
		"username": request.Username,
		"password": request.Password,
	})
	now := s.now().UTC()
	repo := Repository{
		Name:            request.Name,
		DisplayName:     request.DisplayName,
		URL:             request.URL,
		CredentialsJSON: JSON(creds),
		CreatedBy:       &actor.ID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repository.SaveRepo(ctx, &repo); err != nil {
		if isUniqueViolation(err) {
			return Repository{}, ErrRepoNameExists
		}
		return Repository{}, err
	}
	return repo, nil
}

func (s *Service) GetRepository(ctx context.Context, id int64) (Repository, error) {
	return s.repository.GetRepo(ctx, id)
}

func (s *Service) ListRepositories(ctx context.Context) ([]Repository, error) {
	return s.repository.ListRepos(ctx)
}

func (s *Service) DeleteRepository(ctx context.Context, id int64) error {
	return s.repository.DeleteRepo(ctx, id)
}

// RepositoryView converts a Repository to its API-safe projection (no
// credentials).
func RepositoryViewFrom(repo Repository) RepositoryView {
	return RepositoryView{
		ID:          repo.ID,
		Name:        repo.Name,
		DisplayName: repo.DisplayName,
		URL:         repo.URL,
		HasAuth:     hasCredentials(repo.CredentialsJSON),
		CreatedAt:   repo.CreatedAt,
		UpdatedAt:   repo.UpdatedAt,
	}
}

func hasCredentials(creds JSON) bool {
	if len(creds) == 0 || bytes.Equal(creds, []byte("{}")) || bytes.Equal(creds, []byte("null")) {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(creds, &m); err != nil {
		return false
	}
	if u, _ := m["username"].(string); u != "" {
		return true
	}
	if p, _ := m["password"].(string); p != "" {
		return true
	}
	return false
}

func validateRepoRequest(request CreateRepositoryRequest) error {
	name := strings.TrimSpace(request.Name)
	if !validRepoName(name) {
		return ErrInvalidRequest
	}
	rawURL := strings.TrimSpace(request.URL)
	if !validRepoURL(rawURL) {
		return ErrRepoURLInvalid
	}
	if len(request.DisplayName) > 128 {
		return ErrInvalidRequest
	}
	return nil
}

func validRepoName(name string) bool {
	return name != "" && len(name) <= 63 && helmRepoNamePattern.MatchString(name)
}

func validRepoURL(raw string) bool {
	if len(raw) < 8 || len(raw) > 512 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// ---------------------------------------------------------------------------
// Chart listing / detail (read-only index.yaml fetch)
// ---------------------------------------------------------------------------

// ListCharts fetches the repo's index.yaml and returns a flat list of the
// latest version of each chart.
func (s *Service) ListCharts(ctx context.Context, repoID int64) ([]ChartSummary, error) {
	repo, index, err := s.loadIndex(ctx, repoID)
	if err != nil {
		return nil, err
	}
	summaries := make([]ChartSummary, 0, len(index.Entries))
	for name, entries := range index.Entries {
		if len(entries) == 0 {
			continue
		}
		latest := entries[0]
		summaries = append(summaries, ChartSummary{
			Name:        name,
			Version:     latest.Version,
			Description: latest.Description,
			Icon:        latest.Icon,
			Home:        latest.Home,
			AppVersion:  latest.AppVersion,
		})
	}
	_ = repo
	return summaries, nil
}

// GetChart fetches the repo's index.yaml and returns all versions of the
// requested chart.
func (s *Service) GetChart(ctx context.Context, repoID int64, chartName string) (ChartDetail, error) {
	if strings.TrimSpace(chartName) == "" {
		return ChartDetail{}, ErrChartNotFound
	}
	repo, index, err := s.loadIndex(ctx, repoID)
	if err != nil {
		return ChartDetail{}, err
	}
	entries, ok := index.Entries[chartName]
	if !ok || len(entries) == 0 {
		return ChartDetail{}, ErrChartNotFound
	}
	detail := ChartDetail{
		Name:        chartName,
		Description: entries[0].Description,
		Icon:        entries[0].Icon,
		Home:        entries[0].Home,
		Versions:    make([]ChartVersion, 0, len(entries)),
	}
	for _, entry := range entries {
		detail.Versions = append(detail.Versions, ChartVersion{
			Version:    entry.Version,
			AppVersion: entry.AppVersion,
			Created:    entry.Created,
			Digest:     entry.Digest,
		})
	}
	for _, m := range entries[0].Maintainers {
		if m.Name != "" {
			detail.Maintainers = append(detail.Maintainers, m.Name)
		}
	}
	_ = repo
	return detail, nil
}

func (s *Service) loadIndex(ctx context.Context, repoID int64) (Repository, helmIndex, error) {
	repo, err := s.repository.GetRepo(ctx, repoID)
	if err != nil {
		return Repository{}, helmIndex{}, err
	}
	username, password := extractCredentials(repo.CredentialsJSON)
	body, err := s.index.FetchIndex(ctx, repo.URL, username, password)
	if err != nil {
		return Repository{}, helmIndex{}, err
	}
	var index helmIndex
	if err := yaml.Unmarshal(body, &index); err != nil {
		return Repository{}, helmIndex{}, fmt.Errorf("%w: invalid index.yaml: %v", ErrRepoUnreachable, err)
	}
	return repo, index, nil
}

func extractCredentials(creds JSON) (username, password string) {
	if len(creds) == 0 {
		return "", ""
	}
	var m map[string]string
	if err := json.Unmarshal(creds, &m); err != nil {
		return "", ""
	}
	return m["username"], m["password"]
}

// ---------------------------------------------------------------------------
// Deploy preview + execute (M19 controlled-operation contract)
// ---------------------------------------------------------------------------

// Preview validates the deploy request, fetches chart metadata from the repo
// index, builds a Flux HelmRelease CR manifest, runs a server-side dry-run on
// the target cluster, and persists the plan with a one-time confirmation token.
func (s *Service) Preview(ctx context.Context, request DeployPreviewRequest, actor ActorRef) (Plan, error) {
	if err := validateDeployRequest(request); err != nil {
		return Plan{}, err
	}
	if actor.ID < 1 || strings.TrimSpace(actor.Name) == "" {
		return Plan{}, ErrInvalidRequest
	}
	// Verify target namespace exists.
	exists, err := s.kubernetes.NamespaceExists(ctx, request.TargetClusterID, request.TargetNamespace)
	if err != nil {
		return Plan{}, fmt.Errorf("%w: %v", ErrClusterUnavailable, err)
	}
	if !exists {
		return Plan{}, fmt.Errorf("%w: %s", ErrNamespaceMissing, request.TargetNamespace)
	}
	// Fetch chart metadata from repo index for a consistent snapshot.
	repo, index, err := s.loadIndex(ctx, request.RepoID)
	if err != nil {
		return Plan{}, err
	}
	chartEntry, err := findChartEntry(index, request.ChartName, request.ChartVersion)
	if err != nil {
		return Plan{}, err
	}
	// Check that a HelmRelease with the same name does not already exist.
	helmReleasePath := helmReleasePath(request.TargetNamespace, request.ReleaseName)
	exists, err = s.kubernetes.ResourceExists(ctx, request.TargetClusterID, helmReleasePath)
	if err != nil {
		return Plan{}, fmt.Errorf("%w: %v", ErrClusterUnavailable, err)
	}
	if exists {
		return Plan{}, fmt.Errorf("%w: HelmRelease %s/%s", ErrPreviewFailed, request.TargetNamespace, request.ReleaseName)
	}
	// Build the HelmRelease CR manifest.
	manifest, chartMeta, err := buildHelmReleaseManifest(request, repo, chartEntry)
	if err != nil {
		return Plan{}, err
	}
	// Dry-run validate on the target cluster.
	createPath := helmReleaseCreatePath(request.TargetNamespace)
	if _, err := s.kubernetes.CreateResource(ctx, request.TargetClusterID, createPath, manifest, true); err != nil {
		if errors.Is(err, kubernetes.ErrResourceConflict) {
			return Plan{}, fmt.Errorf("%w: HelmRelease %s/%s already exists", ErrPreviewFailed, request.TargetNamespace, request.ReleaseName)
		}
		return Plan{}, fmt.Errorf("%w: %v", ErrPreviewFailed, err)
	}
	// Build the deploy diff shown to the operator.
	diff := buildDeployDiff(request)
	// Generate plan identity (UUID + confirmation token + hash).
	id, token, tokenHash, err := newIdentity()
	if err != nil {
		return Plan{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	plan := Plan{
		ID:                    id,
		Status:                StatusAwaitingConfirmation,
		RepoID:                request.RepoID,
		ChartName:             request.ChartName,
		ChartVersion:          request.ChartVersion,
		TargetClusterID:       request.TargetClusterID,
		TargetNamespace:       request.TargetNamespace,
		ReleaseName:           request.ReleaseName,
		ValuesYAML:            request.ValuesYAML,
		ChartMetadata:         chartMeta,
		ReleaseManifest:       JSON(manifest),
		DeployDiff:            mustMarshal(diff),
		ConfirmationTokenHash: tokenHash,
		RequestedByUserID:     &actor.ID,
		RequestedByName:       actor.Name,
		ExpiresAt:             now.Add(s.planTTL),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := s.repository.SavePlan(ctx, &plan); err != nil {
		return Plan{}, err
	}
	plan.ConfirmationToken = token
	return plan, nil
}

// Execute confirms the plan and creates the HelmRelease CR on the target
// cluster. Idempotent replay with the same idempotency key returns the
// persisted plan without re-applying.
func (s *Service) Execute(ctx context.Context, id, confirmationToken, idempotencyKey string) (Plan, error) {
	id = strings.TrimSpace(id)
	confirmationToken = strings.TrimSpace(confirmationToken)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if id == "" || confirmationToken == "" {
		return Plan{}, ErrConfirmationInvalid
	}
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return Plan{}, ErrInvalidIdempotency
	}
	tokenHash := sha256.Sum256([]byte(confirmationToken))
	now := s.now().UTC()
	plan, shouldExecute, err := s.repository.ClaimPlan(ctx, id, tokenHash[:], idempotencyKey, now, now.Add(-s.claimTTL))
	if err != nil || !shouldExecute {
		return plan, err
	}
	// Apply the HelmRelease CR. The manifest was built and dry-run validated at
	// preview time; we apply it verbatim (deterministic — no re-rendering).
	createPath := helmReleaseCreatePath(plan.TargetNamespace)
	if _, err := s.kubernetes.CreateResource(ctx, plan.TargetClusterID, createPath, []byte(plan.ReleaseManifest), false); err != nil {
		if errors.Is(err, kubernetes.ErrResourceConflict) {
			// The HelmRelease already exists. This is a legitimate idempotent
			// outcome if a previous attempt timed out after creating the CR
			// but before completing the plan. Treat as success.
			completed, completeErr := s.repository.CompletePlan(ctx, plan.ID, idempotencyKey, s.now().UTC())
			if completeErr != nil {
				return Plan{}, completeErr
			}
			return completed, nil
		}
		message := safeExecutionError(err)
		failed, failErr := s.repository.FailPlan(ctx, plan.ID, idempotencyKey, message)
		if failErr != nil {
			return Plan{}, failErr
		}
		return failed, fmt.Errorf("%w: %s", ErrExecutionFailed, message)
	}
	completed, err := s.repository.CompletePlan(ctx, plan.ID, idempotencyKey, s.now().UTC())
	if err != nil {
		return Plan{}, err
	}
	return completed, nil
}

func (s *Service) GetPlan(ctx context.Context, id string) (Plan, error) {
	return s.repository.GetPlan(ctx, id)
}

func (s *Service) ListPlans(ctx context.Context, clusterID int64, namespace string) ([]Plan, error) {
	if clusterID < 1 {
		return nil, ErrInvalidRequest
	}
	return s.repository.ListPlans(ctx, clusterID, namespace)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func validateDeployRequest(request DeployPreviewRequest) error {
	if request.RepoID < 1 {
		return ErrInvalidRequest
	}
	if request.TargetClusterID < 1 {
		return ErrInvalidRequest
	}
	if !validName(request.TargetNamespace, 63) {
		return ErrInvalidRequest
	}
	if !validName(request.ReleaseName, 253) {
		return ErrInvalidRequest
	}
	if strings.TrimSpace(request.ChartName) == "" || len(request.ChartName) > 253 {
		return ErrInvalidRequest
	}
	if strings.TrimSpace(request.ChartVersion) == "" || len(request.ChartVersion) > 128 {
		return ErrInvalidRequest
	}
	if len(request.ValuesYAML) > 256<<10 {
		return ErrInvalidRequest
	}
	return nil
}

func validName(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && kubernetesNamePattern.MatchString(value)
}

func findChartEntry(index helmIndex, name, version string) (helmIndexEntry, error) {
	entries, ok := index.Entries[name]
	if !ok || len(entries) == 0 {
		return helmIndexEntry{}, ErrChartNotFound
	}
	for _, entry := range entries {
		if entry.Version == version {
			return entry, nil
		}
	}
	return helmIndexEntry{}, ErrChartNotFound
}

// buildHelmReleaseManifest constructs the Flux HelmRelease CR JSON that will
// be applied at execute time. The manifest is built once at preview and
// applied verbatim at execute (deterministic — no re-rendering).
func buildHelmReleaseManifest(request DeployPreviewRequest, repo Repository, entry helmIndexEntry) (json.RawMessage, JSON, error) {
	values := map[string]any{}
	if strings.TrimSpace(request.ValuesYAML) != "" {
		if err := yaml.Unmarshal([]byte(request.ValuesYAML), &values); err != nil {
			return nil, nil, fmt.Errorf("%w: values_yaml is not valid YAML: %v", ErrInvalidRequest, err)
		}
	}
	chartMeta := map[string]any{
		"name":        entry.Name,
		"version":     entry.Version,
		"app_version": entry.AppVersion,
		"description": entry.Description,
		"home":        entry.Home,
		"icon":        entry.Icon,
	}
	manifest := map[string]any{
		"apiVersion": helmReleaseAPIVersion,
		"kind":       helmReleaseKind,
		"metadata": map[string]any{
			"name":      request.ReleaseName,
			"namespace": request.TargetNamespace,
		},
		"spec": map[string]any{
			"chart": map[string]any{
				"spec": map[string]any{
					"chart":   request.ChartName,
					"version": request.ChartVersion,
					"sourceRef": map[string]any{
						"kind":      "HelmRepository",
						"name":      repo.Name,
						"namespace": request.TargetNamespace,
					},
				},
			},
			"values": values,
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("encode HelmRelease manifest: %w", err)
	}
	metaData, _ := json.Marshal(chartMeta)
	return json.RawMessage(data), JSON(metaData), nil
}

func buildDeployDiff(request DeployPreviewRequest) DeployDiff {
	diff := DeployDiff{
		Mode:         "create",
		ChartName:    request.ChartName,
		ChartVersion: request.ChartVersion,
		Namespace:    request.TargetNamespace,
		ReleaseName:  request.ReleaseName,
	}
	if strings.TrimSpace(request.ValuesYAML) != "" {
		var values map[string]any
		if err := yaml.Unmarshal([]byte(request.ValuesYAML), &values); err == nil {
			diff.Values = values
		}
	}
	return diff
}

// helmReleaseCreatePath returns the Kubernetes API path for creating a
// HelmRelease CR in the given namespace.
func helmReleaseCreatePath(namespace string) string {
	return "/apis/" + helmReleaseAPIVersion + "/namespaces/" + url.PathEscape(namespace) + "/" + helmReleasePlural
}

// helmReleasePath returns the Kubernetes API path for a specific HelmRelease CR.
func helmReleasePath(namespace, name string) string {
	return helmReleaseCreatePath(namespace) + "/" + url.PathEscape(name)
}

func safeExecutionError(err error) string {
	text := err.Error()
	if len(text) > 480 {
		text = text[:480]
	}
	return text
}

func mustMarshal(value any) JSON {
	body, err := json.Marshal(value)
	if err != nil {
		return JSON("null")
	}
	return JSON(body)
}

func newIdentity() (id, token string, tokenHash []byte, err error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", "", nil, err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	id = hex.EncodeToString(raw[0:4]) + "-" + hex.EncodeToString(raw[4:6]) + "-" + hex.EncodeToString(raw[6:8]) + "-" + hex.EncodeToString(raw[8:10]) + "-" + hex.EncodeToString(raw[10:16])
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(token))
	return id, token, hash[:], nil
}

// isUniqueViolation checks if a GORM error is a unique constraint violation.
// This is driver-agnostic: it checks for common unique-violation substrings.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "UNIQUE constraint failed")
}
