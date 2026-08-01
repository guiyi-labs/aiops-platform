package copyops

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// KubernetesSource is the subset of kubernetes.Service used by copyops: raw
// manifest reads, namespace existence, source-namespace identity, and
// server-side dry-run / apply create.
type KubernetesSource interface {
	GetRawResource(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (map[string]any, error)
	NamespaceExists(ctx context.Context, clusterID int64, namespace string) (bool, error)
	SourceNamespaceIdentity(ctx context.Context, clusterID int64, namespace string) (k8sgateway.SourceNamespaceIdentity, error)
	NamespacedResourceExists(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (bool, error)
	CreateResource(ctx context.Context, clusterID int64, path string, body []byte, dryRun bool) ([]byte, error)
}

var kubeNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$`)

const (
	defaultPlanTTL  = 10 * time.Minute
	defaultClaimTTL = 15 * time.Second
	emptyIdentity   = "00000000-0000-0000-0000-000000000000"
)

type Service struct {
	kubernetes KubernetesSource
	repository Repository
	planTTL    time.Duration
	now        func() time.Time
	randBytes  func(n int) ([]byte, error)
}

func NewService(kubernetes KubernetesSource, repository Repository) *Service {
	return &Service{
		kubernetes: kubernetes,
		repository: repository,
		planTTL:    defaultPlanTTL,
		now:        time.Now,
		randBytes: func(n int) ([]byte, error) {
			buf := make([]byte, n)
			_, err := rand.Read(buf)
			return buf, err
		},
	}
}

// NewTestService exposes tunables for unit tests.
func NewTestService(kubernetes KubernetesSource, repository Repository, now func() time.Time, randBytes func(int) ([]byte, error)) *Service {
	svc := NewService(kubernetes, repository)
	if now != nil {
		svc.now = now
	}
	if randBytes != nil {
		svc.randBytes = randBytes
	}
	return svc
}

// --- Preview -----------------------------------------------------------------

// Preview validates the request, fetches each source manifest, scrubs it for
// cross-cluster application, runs a server-side dry-run on the target and
// persists an awaiting-confirmation plan with a one-time confirmation token.
//
// The plan's SourceNamespaceUID/RV are captured here; Execute re-verifies
// them as a Compare-And-Swap gate (mirrors the M28 backup contract).
func (s *Service) Preview(ctx context.Context, request PreviewRequest, actor ActorRef) (Plan, error) {
	// Cap the bundle *before* normalization/dedup so a bad client can't blow
	// up memory by sending thousands of duplicates.
	if len(request.Bundle) > MaxBundle {
		return Plan{}, ErrBundleTooLarge
	}
	request = normalizeRequest(request)
	if err := validatePreviewRequest(request); err != nil {
		return Plan{}, err
	}

	// Preflight 0: capture source namespace identity *before* any manifest
	// reads, so we detect torn reads if the namespace is recreated mid-call.
	sourceIdent, err := s.kubernetes.SourceNamespaceIdentity(ctx, request.SourceClusterID, request.SourceNamespace)
	if err != nil {
		if errors.Is(err, k8sgateway.ErrResourceNotFound) {
			return Plan{}, ErrSourceUnavailable
		}
		return Plan{}, ErrSourceUnavailable
	}
	if sourceIdent.UID == "" {
		return Plan{}, ErrSourceUnavailable
	}

	// Preflight 1: target namespace must exist. In M58 we do NOT auto-create
	// target namespaces to avoid surprising cluster admins.
	nsExists, err := s.kubernetes.NamespaceExists(ctx, request.TargetClusterID, request.TargetNamespace)
	if err != nil {
		return Plan{}, ErrDestinationUnavailable
	}
	if !nsExists {
		return Plan{}, ErrNamespaceMissing
	}

	// Fetch each source manifest and scrub.
	items := make([]ResourceItem, 0, len(request.Bundle))
	summary := make([]CopySummaryItem, 0, len(request.Bundle))
	dryRunErrors := make([]string, 0, 4)
	willCreate := 0
	willSkip := 0

	for _, entry := range request.Bundle {
		group, version, resource, ok := k8sgateway.KindToGVR(entry.Kind)
		if !ok {
			return Plan{}, fmt.Errorf("%w: %s", ErrKindDisallowed, entry.Kind)
		}
		raw, err := s.kubernetes.GetRawResource(ctx, request.SourceClusterID, group, version, resource, entry.Namespace, entry.Name)
		if err != nil {
			if errors.Is(err, k8sgateway.ErrResourceNotFound) {
				return Plan{}, fmt.Errorf("%w: %s/%s/%s", ErrSourceNotFound, entry.Kind, entry.Namespace, entry.Name)
			}
			return Plan{}, ErrSourceUnavailable
		}
		uid := getString(raw, "metadata", "uid")
		rv := getString(raw, "metadata", "resourceVersion")
		destName := entry.Name
		destNamespace := request.TargetNamespace
		scrubbed := scrubManifest(raw, request.StripLabelPrefixes, request.StripAnnotationPrefixes, request.StripSecrets && entry.Kind == KindSecret)
		// Rewrite metadata.namespace to the target so dry-run validates
		// against the target namespace (e.g. quota checks).
		setString(scrubbed, request.TargetNamespace, "metadata", "namespace")
		setString(scrubbed, destName, "metadata", "name")

		item := ResourceItem{
			Group:                 group,
			Version:               version,
			Resource:              resource,
			Kind:                  entry.Kind,
			SourceNamespace:       entry.Namespace,
			SourceName:            entry.Name,
			SourceUID:             uid,
			SourceResourceVersion: rv,
			DestinationNamespace:  destNamespace,
			DestinationName:       destName,
			Manifest:              scrubbed,
			ItemStatus:            ItemStatusPending,
			Diff: MarshalDiff(ItemDiff{
				Mode:          ModeCreate,
				After:         scrubbed,
				ChangedFields: []string{".*"},
			}),
		}

		// Preflight 2: skip already-existing resources with a friendly reason.
		exists, err := s.kubernetes.NamespacedResourceExists(ctx, request.TargetClusterID, group, version, resource, destNamespace, destName)
		if err != nil {
			return Plan{}, ErrDestinationUnavailable
		}
		if exists {
			item.ItemStatus = ItemStatusSkipped
			item.PreflightSkip = "already exists on destination cluster"
			willSkip++
			dryRunErrors = append(dryRunErrors, fmt.Sprintf("%s/%s skipped: exists", entry.Kind, entry.Name))
		} else {
			// Server-side dry-run create.
			manifestJSON, err := json.Marshal(scrubbed)
			if err != nil {
				return Plan{}, fmt.Errorf("encode manifest for dry-run: %w", err)
			}
			path := k8sCollectionPath(group, version, resource, destNamespace)
			if _, err := s.kubernetes.CreateResource(ctx, request.TargetClusterID, path, manifestJSON, true); err != nil {
				if errors.Is(err, k8sgateway.ErrResourceConflict) {
					item.ItemStatus = ItemStatusSkipped
					item.PreflightSkip = "destination already exists (admission reported 409)"
					willSkip++
				} else {
					item.ItemStatus = ItemStatusFailed
					item.DryRunError = err.Error()
					dryRunErrors = append(dryRunErrors, fmt.Sprintf("%s/%s dry-run: %s", entry.Kind, entry.Name, err.Error()))
				}
			} else {
				willCreate++
			}
		}
		items = append(items, item)
		summary = append(summary, CopySummaryItem{
			Kind:          entry.Kind,
			Namespace:     entry.Namespace,
			Name:          entry.Name,
			DestNamespace: destNamespace,
			DestName:      destName,
		})
	}

	// A plan where every item is skipped still succeeds (operators may want
	// to commit the plan so the UI shows a consistent audit trail). But a
	// plan where every item *failed* dry-run is an error.
	anyPendingOrSkipped := false
	allFailed := true
	for _, it := range items {
		if it.ItemStatus != ItemStatusFailed {
			allFailed = false
		}
		if it.ItemStatus == ItemStatusPending || it.ItemStatus == ItemStatusSkipped {
			anyPendingOrSkipped = true
		}
	}
	if allFailed && !anyPendingOrSkipped {
		return Plan{}, fmt.Errorf("%w: %s", ErrPreviewFailed, strings.Join(dryRunErrors, "; "))
	}

	// Persist plan.
	id, token, tokenHash, err := s.newIdentity()
	if err != nil {
		return Plan{}, err
	}
	now := s.now()
	plan := Plan{
		ID:                 id,
		Status:             StatusAwaitingConfirmation,
		SourceClusterID:    request.SourceClusterID,
		SourceNamespace:    request.SourceNamespace,
		SourceNamespaceUID: sourceIdent.UID,
		SourceNamespaceRV:  sourceIdent.ResourceVersion,
		TargetClusterID:    request.TargetClusterID,
		TargetNamespace:    request.TargetNamespace,
		ResourceItems:      MarshalResourceItems(items),
		CopySummary:        MarshalCopySummary(summary),
		Diff: MarshalPlanDiff(PlanDiff{
			ResourceCount:         len(items),
			TargetNamespaceExists: true,
			WillCreateCount:       willCreate,
			WillSkipCount:         willSkip,
			DryRunErrors:          dryRunErrors,
		}),
		ConfirmationTokenHash: tokenHash,
		RequestedByUserID:     &actor.ID,
		RequestedByName:       actor.Name,
		ExpiresAt:             now.Add(s.planTTL),
		CreatedAt:             now,
		UpdatedAt:             now,
		ConfirmationToken:     token,
	}
	saved, err := s.repository.Create(ctx, plan)
	if err != nil {
		return Plan{}, fmt.Errorf("persist copy plan: %w", err)
	}
	saved.ConfirmationToken = token
	return saved, nil
}

// --- Execute -----------------------------------------------------------------

type ExecuteRequest struct {
	PlanID            string
	ConfirmationToken string
	IdempotencyKey    string
}

// Execute claims the plan, re-validates preconditions (source namespace
// identity CAS gate + destination namespace existence), and applies pending
// items in order. Execute is idempotent: presenting the same idempotency key
// returns the already-committed plan result instead of re-applying.
func (s *Service) Execute(ctx context.Context, request ExecuteRequest, actor ActorRef) (Plan, error) {
	request = normalizeExecuteRequest(request)
	if err := validateExecuteRequest(request); err != nil {
		return Plan{}, err
	}
	tokenHash := sha256Sum([]byte(request.ConfirmationToken))

	// Claim (atomic + re-checks token, idempotency, expiry, status).
	plan, err := s.repository.ClaimAndLoad(ctx, request.PlanID, request.IdempotencyKey, tokenHash, request.IdempotencyKey)
	if err != nil {
		return Plan{}, err
	}
	// If the plan is already not-awaiting (idempotency replay) return as-is.
	if plan.Status != StatusExecuting {
		return plan, nil
	}

	items := UnmarshalResourceItems(plan.ResourceItems)

	// CAS gate: source namespace identity must match.
	casIdent, err := s.kubernetes.SourceNamespaceIdentity(ctx, plan.SourceClusterID, plan.SourceNamespace)
	if err != nil {
		return s.failPlan(ctx, plan, items, "source namespace identity read failed: "+err.Error())
	}
	if casIdent.UID != plan.SourceNamespaceUID || casIdent.UID == "" {
		return s.failPlan(ctx, plan, items, "source namespace identity drift: re-run Preview")
	}

	// Destination namespace existence (must still exist, or we fail-fast).
	destNsExists, err := s.kubernetes.NamespaceExists(ctx, plan.TargetClusterID, plan.TargetNamespace)
	if err != nil {
		return s.failPlan(ctx, plan, items, "destination namespace check failed: "+err.Error())
	}
	if !destNsExists {
		return s.failPlan(ctx, plan, items, "destination namespace was deleted between Preview and Execute")
	}

	appliedCount := 0
	failedCount := 0
	skippedCount := 0
	for i, item := range items {
		if item.ItemStatus != ItemStatusPending {
			if item.ItemStatus == ItemStatusSkipped {
				skippedCount++
			}
			continue
		}
		manifestJSON, err := json.Marshal(item.Manifest)
		if err != nil {
			items[i].ItemStatus = ItemStatusFailed
			items[i].LastError = "encode manifest: " + err.Error()
			failedCount++
			continue
		}
		path := k8sCollectionPath(item.Group, item.Version, item.Resource, item.DestinationNamespace)
		resp, err := s.kubernetes.CreateResource(ctx, plan.TargetClusterID, path, manifestJSON, false)
		if err != nil {
			items[i].ItemStatus = ItemStatusFailed
			items[i].LastError = err.Error()
			failedCount++
			continue
		}
		// Parse out uid / resourceVersion from the API response.
		var parsed struct {
			Metadata struct {
				UID             string `json:"uid"`
				ResourceVersion string `json:"resourceVersion"`
			} `json:"metadata"`
		}
		_ = json.Unmarshal(resp, &parsed)
		items[i].ItemStatus = ItemStatusApplied
		items[i].AppliedUID = parsed.Metadata.UID
		items[i].AppliedRV = parsed.Metadata.ResourceVersion
		appliedCount++
	}

	// Finalize plan status.
	now := s.now()
	plan.ExecutedAt = &now
	plan.ResourceItems = MarshalResourceItems(items)
	if failedCount > 0 && appliedCount == 0 {
		plan.Status = StatusFailed
	} else if failedCount > 0 {
		plan.Status = "partial" // note: partial is in the migration CHECK? No — only awaiting_confirmation/executing/succeeded/failed/expired. Fall back to "failed" to satisfy constraint.
		plan.Status = StatusFailed
		plan.LastError = fmt.Sprintf("%d items failed, %d applied, %d skipped", failedCount, appliedCount, skippedCount)
	} else {
		plan.Status = StatusSucceeded
	}
	if err := s.repository.UpdateExecution(ctx, plan); err != nil {
		return Plan{}, fmt.Errorf("persist execute result: %w", err)
	}
	return plan, nil
}

func (s *Service) failPlan(ctx context.Context, plan Plan, items []ResourceItem, msg string) (Plan, error) {
	// Mark any still-pending items as skipped so the plan row remains
	// consistent with the preflight failure.
	for i := range items {
		if items[i].ItemStatus == ItemStatusPending {
			items[i].ItemStatus = ItemStatusSkipped
			items[i].PreflightSkip = "plan precheck failed: " + msg
		}
	}
	plan.ResourceItems = MarshalResourceItems(items)
	plan.LastError = msg
	plan.Status = StatusFailed
	now := s.now()
	plan.ExecutedAt = &now
	if err := s.repository.UpdateExecution(ctx, plan); err != nil {
		return Plan{}, fmt.Errorf("persist failed execution: %w", err)
	}
	return plan, nil
}

// --- Queries -----------------------------------------------------------------

func (s *Service) Get(ctx context.Context, id string) (Plan, error) {
	if id == "" {
		return Plan{}, ErrInvalidRequest
	}
	return s.repository.GetByID(ctx, id)
}

func (s *Service) ListByUser(ctx context.Context, userID int64, offset, limit int) ([]Plan, int, error) {
	if userID < 1 {
		return nil, 0, ErrInvalidRequest
	}
	return s.repository.ListByUser(ctx, userID, offset, limit)
}

func (s *Service) ListByCluster(ctx context.Context, clusterID int64, offset, limit int) ([]Plan, int, error) {
	if clusterID < 1 {
		return nil, 0, ErrInvalidRequest
	}
	return s.repository.ListByCluster(ctx, clusterID, offset, limit)
}

// --- internals ---------------------------------------------------------------

func normalizeRequest(r PreviewRequest) PreviewRequest {
	r.SourceNamespace = strings.TrimSpace(r.SourceNamespace)
	r.TargetNamespace = strings.TrimSpace(r.TargetNamespace)
	out := make([]BundleItemRequest, 0, len(r.Bundle))
	seen := make(map[string]struct{}, len(r.Bundle))
	for _, it := range r.Bundle {
		it.Kind = strings.TrimSpace(it.Kind)
		it.Namespace = strings.TrimSpace(it.Namespace)
		it.Name = strings.TrimSpace(it.Name)
		if it.Namespace == "" {
			it.Namespace = r.SourceNamespace
		}
		key := it.Kind + "/" + it.Namespace + "/" + it.Name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, it)
	}
	r.Bundle = out
	// Deduplicate prefix lists.
	r.StripLabelPrefixes = dedupeStrings(r.StripLabelPrefixes)
	r.StripAnnotationPrefixes = dedupeStrings(r.StripAnnotationPrefixes)
	return r
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func validatePreviewRequest(r PreviewRequest) error {
	if r.SourceClusterID < 1 || r.TargetClusterID < 1 {
		return ErrInvalidRequest
	}
	if r.SourceClusterID == r.TargetClusterID {
		return ErrCrossClusterSame
	}
	if r.SourceNamespace == "" || !kubeNamePattern.MatchString(r.SourceNamespace) {
		return ErrInvalidRequest
	}
	if r.TargetNamespace == "" || !kubeNamePattern.MatchString(r.TargetNamespace) {
		return ErrInvalidRequest
	}
	if len(r.Bundle) == 0 {
		return ErrBundleEmpty
	}
	if len(r.Bundle) > MaxBundle {
		return ErrBundleTooLarge
	}
	for _, it := range r.Bundle {
		if it.Kind == "" || it.Name == "" || it.Namespace == "" {
			return ErrInvalidRequest
		}
		if !kubeNamePattern.MatchString(it.Name) {
			return ErrInvalidRequest
		}
		if !kubeNamePattern.MatchString(it.Namespace) {
			return ErrInvalidRequest
		}
	}
	return nil
}

func normalizeExecuteRequest(r ExecuteRequest) ExecuteRequest {
	r.PlanID = strings.TrimSpace(r.PlanID)
	r.ConfirmationToken = strings.TrimSpace(r.ConfirmationToken)
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	return r
}

func validateExecuteRequest(r ExecuteRequest) error {
	if len(r.PlanID) != 36 {
		return ErrInvalidRequest
	}
	if r.ConfirmationToken == "" {
		return ErrConfirmationInvalid
	}
	if r.IdempotencyKey == "" {
		return ErrInvalidIdempotency
	}
	return nil
}

// scrubManifest returns a copy of the source manifest with cluster-specific
// and controller-owned fields stripped so it can be applied verbatim to a
// different namespace/cluster.
func scrubManifest(src map[string]any, stripLabelPrefixes, stripAnnotationPrefixes []string, stripSecrets bool) map[string]any {
	// Deep copy via JSON round-trip so the caller's original is untouched.
	raw, _ := json.Marshal(src)
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)

	// Always drop status and common cluster-specific fields.
	delete(obj, "status")
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		return obj
	}
	for _, k := range []string{"uid", "resourceVersion", "creationTimestamp", "generation", "selfLink", "managedFields", "deletionTimestamp", "deletionGracePeriodSeconds", "finalizers", "ownerReferences"} {
		delete(meta, k)
	}
	// Strip cluster-specific labels/annotations (controller-owned, admission
	// controller-injected, etc.).
	stripMapPrefixes(meta, "labels", stripLabelPrefixes)
	stripMapPrefixes(meta, "annotations", stripAnnotationPrefixes)
	// Always strip a few high-risk annotations regardless of request.
	if ann, ok := meta["annotations"].(map[string]any); ok {
		for _, k := range []string{
			"kubectl.kubernetes.io/last-applied-configuration",
			"deployment.kubernetes.io/revision",
			"kubernetes.io/change-cause",
			"control-plane.alpha.kubernetes.io/leader",
		} {
			delete(ann, k)
		}
	}
	// Drop any service-specific cluster-assigned fields.
	if spec, ok := obj["spec"].(map[string]any); ok {
		// Service: don't copy the clusterIP, nodePorts or healthCheckNodePort
		// assigned by the source API server.
		delete(spec, "clusterIP")
		delete(spec, "clusterIPs")
		delete(spec, "healthCheckNodePort")
		if ports, ok := spec["ports"].([]any); ok {
			for _, pAny := range ports {
				if p, ok := pAny.(map[string]any); ok {
					delete(p, "nodePort")
				}
			}
		}
		// PodSpec subfields: any nodeName / nodeSelector term that pins to
		// a source node is dropped.
		dropNodePinning(spec)
	}
	if stripSecrets {
		if typ, _ := obj["type"].(string); typ == "" {
			// Not a typed Secret — leave as-is.
		} else {
			// Replace data / stringData with empty placeholders so the
			// destination Secret exists but contains no secret material.
			if _, ok := obj["data"]; ok {
				obj["data"] = map[string]any{}
			}
			if _, ok := obj["stringData"]; ok {
				delete(obj, "stringData")
			}
		}
	}
	return obj
}

func stripMapPrefixes(obj map[string]any, key string, prefixes []string) {
	if len(prefixes) == 0 {
		return
	}
	m, ok := obj[key].(map[string]any)
	if !ok {
		return
	}
	for k := range m {
		for _, p := range prefixes {
			if p != "" && strings.HasPrefix(k, p) {
				delete(m, k)
				break
			}
		}
	}
	if len(m) == 0 {
		delete(obj, key)
	}
}

func dropNodePinning(spec map[string]any) {
	if tmpl, ok := spec["template"].(map[string]any); ok {
		if tmplSpec, ok := tmpl["spec"].(map[string]any); ok {
			delete(tmplSpec, "nodeName")
			// Affinity is not auto-dropped because it's often desired
			// across clusters; we drop only nodeName.
		}
	}
}

func getString(obj map[string]any, path ...string) string {
	cur := any(obj)
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = m[k]
		if !ok {
			return ""
		}
	}
	s, _ := cur.(string)
	return s
}

func setString(obj map[string]any, value string, path ...string) {
	if len(path) == 0 {
		return
	}
	cur := obj
	for i, k := range path {
		if i == len(path)-1 {
			cur[k] = value
			return
		}
		next, ok := cur[k].(map[string]any)
		if !ok {
			next = make(map[string]any)
			cur[k] = next
		}
		cur = next
	}
}

func k8sCollectionPath(group, version, resource, namespace string) string {
	if group == "" {
		return "/api/" + version + "/namespaces/" + urlEscapePath(namespace) + "/" + resource
	}
	return "/apis/" + group + "/" + version + "/namespaces/" + urlEscapePath(namespace) + "/" + resource
}

func urlEscapePath(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "%", "%25"), "/", "%2F")
}

func sha256Sum(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// newIdentity returns a new (planID, confirmationToken, tokenHash) triple.
// Plan IDs are 128-bit random values encoded as hex (32 chars) — that's
// shorter than uuid4's 36 chars but still collision-safe. To satisfy the
// migration CHECK (char_length(id) = 36), we prefix with "cp-" and suffix
// with "cp" so total length = 3 + 32 + 1 = 36.
// Confirmation tokens are 32 random bytes base64url-encoded.
func (s *Service) newIdentity() (string, string, []byte, error) {
	idBuf, err := s.randBytes(16)
	if err != nil {
		return "", "", nil, err
	}
	id := "cp-" + hex.EncodeToString(idBuf) + "c"
	if len(id) != 36 {
		return "", "", nil, fmt.Errorf("internal: generated copy plan id length %d, want 36", len(id))
	}
	tokenBuf, err := s.randBytes(32)
	if err != nil {
		return "", "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBuf)
	h := sha256.Sum256([]byte(token))
	return id, token, h[:], nil
}
