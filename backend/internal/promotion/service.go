package promotion

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

	"k8s-aiops.local/backend/internal/kubernetes"
)

var kubernetesNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$`)

// ClusterSource lets the promotion service verify that both clusters are
// enabled before preview. It mirrors the subset of cluster.Service the
// promotion flow needs.
type ClusterSource interface {
	Access(ctx context.Context, id int64) (struct{}, error)
}

// KubernetesSource is the subset of kubernetes.Service the promotion flow
// needs. Every method takes an explicit clusterID so the same source can
// serve both the source and destination cluster.
type KubernetesSource interface {
	RawManifest(ctx context.Context, clusterID int64, kind, namespace, name string) (json.RawMessage, error)
	NamespaceExists(ctx context.Context, clusterID int64, namespace string) (bool, error)
	ConfigMapExists(ctx context.Context, clusterID int64, namespace, name string) (bool, error)
	SecretExists(ctx context.Context, clusterID int64, namespace, name string) (bool, error)
	ResourceExists(ctx context.Context, clusterID int64, path string) (bool, error)
	CreateResource(ctx context.Context, clusterID int64, path string, body []byte, dryRun bool) ([]byte, error)
}

type Service struct {
	kubernetes KubernetesSource
	repository Repository
	planTTL    time.Duration
	claimTTL   time.Duration
	now        func() time.Time
}

func NewService(kubernetes KubernetesSource, repository Repository) *Service {
	return &Service{kubernetes: kubernetes, repository: repository, planTTL: 15 * time.Minute, claimTTL: 2 * time.Minute, now: time.Now}
}

// Preview validates the request, runs preflight on the destination cluster,
// strips runtime/server-owned fields from each source manifest, performs a
// server-side dry-run create on the destination, and persists the plan with a
// one-time confirmation token.
func (s *Service) Preview(ctx context.Context, request PreviewRequest, actor ActorRef) (Plan, error) {
	if err := validateRequest(request); err != nil {
		return Plan{}, err
	}
	if actor.ID < 1 || strings.TrimSpace(actor.Name) == "" {
		return Plan{}, ErrInvalidRequest
	}
	if err := s.preflight(ctx, request); err != nil {
		return Plan{}, err
	}
	items := make([]BundleItem, 0, len(request.Bundle))
	allDependencyRefs := make([]DependencyReference, 0, len(request.DependencyMappings))
	for ordinal, requested := range request.Bundle {
		manifest, err := s.kubernetes.RawManifest(ctx, request.SourceClusterID, requested.Kind, requested.Namespace, requested.Name)
		if err != nil {
			return Plan{}, fmt.Errorf("%w: %s %s/%s: %v", ErrSourceUnavailable, requested.Kind, requested.Namespace, requested.Name, err)
		}
		stripped, sourceUID, sourceRV, err := stripRuntimeFields(manifest)
		if err != nil {
			return Plan{}, fmt.Errorf("%w: %s %s/%s: %v", ErrInvalidRequest, requested.Kind, requested.Namespace, requested.Name, err)
		}
		stripped, err = rewriteNamespace(stripped, request.DestinationNamespace)
		if err != nil {
			return Plan{}, err
		}
		refs := scanDependencies(stripped, request.SourceNamespace)
		if err := s.verifyDependencyMappings(ctx, request, refs); err != nil {
			return Plan{}, err
		}
		allDependencyRefs = append(allDependencyRefs, refs...)
		stripped, err = rewriteDependencyMappings(stripped, request.DependencyMappings, request.SourceNamespace)
		if err != nil {
			return Plan{}, err
		}
		diff := buildDiff(requested.Kind, stripped)
		item := BundleItem{
			PlanID:                "",
			Ordinal:               ordinal,
			Kind:                  requested.Kind,
			SourceNamespace:       requested.Namespace,
			SourceName:            requested.Name,
			SourceUID:             sourceUID,
			SourceResourceVersion: sourceRV,
			DestinationNamespace:  request.DestinationNamespace,
			DestinationName:       requested.Name,
			Manifest:              stripped,
			Diff:                  mustMarshal(diff),
			ItemStatus:            ItemStatusPending,
		}
		items = append(items, item)
	}
	dependencies := buildDependencyRecords(request, dedupeRefs(allDependencyRefs))
	for i := range items {
		if err := s.dryRunCreate(ctx, request.DestinationClusterID, items[i]); err != nil {
			if errors.Is(err, kubernetes.ErrResourceConflict) {
				return Plan{}, fmt.Errorf("%w: %s %s/%s", ErrConflict, items[i].Kind, items[i].DestinationNamespace, items[i].DestinationName)
			}
			return Plan{}, fmt.Errorf("%w: %s %s/%s: %v", ErrPreviewFailed, items[i].Kind, items[i].DestinationNamespace, items[i].DestinationName, err)
		}
	}
	id, token, tokenHash, err := newIdentity()
	if err != nil {
		return Plan{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	plan := Plan{
		ID:                    id,
		SourceClusterID:       request.SourceClusterID,
		DestinationClusterID:  request.DestinationClusterID,
		SourceNamespace:       request.SourceNamespace,
		DestinationNamespace:  request.DestinationNamespace,
		Status:                StatusAwaitingConfirmation,
		BundleSummary:         mustMarshal(buildBundleSummary(items)),
		DependencySummary:     mustMarshal(dependencies),
		ConfirmationTokenHash: tokenHash,
		RequestedByUserID:     &actor.ID,
		RequestedByName:       actor.Name,
		ExpiresAt:             now.Add(s.planTTL),
		CreatedAt:             now,
		UpdatedAt:             now,
		Items:                 items,
		Dependencies:          dependencies,
	}
	for i := range plan.Items {
		plan.Items[i].PlanID = id
	}
	for i := range plan.Dependencies {
		plan.Dependencies[i].PlanID = id
	}
	if err := s.repository.Save(ctx, &plan); err != nil {
		return Plan{}, err
	}
	plan.ConfirmationToken = token
	return plan, nil
}

// Execute confirms the plan, applies each bundle item in ordinal order, and
// records per-item outcome. Idempotent replay with the same idempotency key
// returns the persisted plan without re-applying.
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
	plan, shouldExecute, err := s.repository.Claim(ctx, id, tokenHash[:], idempotencyKey, now, now.Add(-s.claimTTL))
	if err != nil || !shouldExecute {
		return plan, err
	}
	itemStatuses := make(map[int64]string)
	itemErrors := make(map[int64]string)
	anyIssue := false
	for _, item := range plan.Items {
		if item.ItemStatus == ItemStatusApplied {
			itemStatuses[item.ID] = ItemStatusApplied
			continue
		}
		path, err := promotionCreatePath(item.Kind, item.DestinationNamespace)
		if err != nil {
			itemStatuses[item.ID] = ItemStatusFailed
			itemErrors[item.ID] = truncate(err.Error())
			anyIssue = true
			continue
		}
		if _, err := s.kubernetes.CreateResource(ctx, plan.DestinationClusterID, path, item.Manifest, false); err != nil {
			if errors.Is(err, kubernetes.ErrResourceConflict) {
				itemStatuses[item.ID] = ItemStatusSkipped
				itemErrors[item.ID] = "destination resource already exists"
				anyIssue = true
				continue
			}
			itemStatuses[item.ID] = ItemStatusFailed
			itemErrors[item.ID] = truncate(safeExecutionError(err))
			anyIssue = true
			continue
		}
		itemStatuses[item.ID] = ItemStatusApplied
	}
	completed, err := s.repository.Complete(ctx, plan.ID, idempotencyKey, s.now().UTC(), itemStatuses, itemErrors)
	if err != nil {
		return Plan{}, err
	}
	if anyIssue {
		return completed, fmt.Errorf("%w: one or more promotion items were not applied", ErrExecutionFailed)
	}
	return completed, nil
}

func (s *Service) Get(ctx context.Context, id string) (Plan, error) {
	return s.repository.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, sourceClusterID int64, namespace string) ([]Plan, error) {
	if sourceClusterID < 1 {
		return nil, ErrInvalidRequest
	}
	return s.repository.List(ctx, sourceClusterID, namespace)
}

func (s *Service) preflight(ctx context.Context, request PreviewRequest) error {
	exists, err := s.kubernetes.NamespaceExists(ctx, request.DestinationClusterID, request.DestinationNamespace)
	if err != nil {
		return fmt.Errorf("%w: destination namespace lookup failed: %v", ErrDestinationUnavailable, err)
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrNamespaceMissing, request.DestinationNamespace)
	}
	for _, item := range request.Bundle {
		path, err := promotionPathForExists(item.Kind, request.DestinationNamespace, item.Name)
		if err != nil {
			return err
		}
		exists, err := s.kubernetes.ResourceExists(ctx, request.DestinationClusterID, path)
		if err != nil {
			return fmt.Errorf("%w: destination conflict check failed: %v", ErrDestinationUnavailable, err)
		}
		if exists {
			return fmt.Errorf("%w: %s %s/%s", ErrConflict, item.Kind, request.DestinationNamespace, item.Name)
		}
	}
	return nil
}

func (s *Service) dryRunCreate(ctx context.Context, destinationClusterID int64, item BundleItem) error {
	path, err := promotionCreatePath(item.Kind, item.DestinationNamespace)
	if err != nil {
		return err
	}
	_, err = s.kubernetes.CreateResource(ctx, destinationClusterID, path, item.Manifest, true)
	return err
}

func (s *Service) verifyDependencyMappings(ctx context.Context, request PreviewRequest, refs []DependencyReference) error {
	providedByKey := make(map[string]bool, len(request.DependencyMappings))
	for _, mapping := range request.DependencyMappings {
		key := dependencyKey(mapping.Kind, mapping.SourceNamespace, mapping.SourceName)
		providedByKey[key] = true
		exists, err := false, error(nil)
		if mapping.Kind == KindConfigMap {
			exists, err = s.kubernetes.ConfigMapExists(ctx, request.DestinationClusterID, mapping.DestinationNamespace, mapping.DestinationName)
		} else if mapping.Kind == KindSecret {
			exists, err = s.kubernetes.SecretExists(ctx, request.DestinationClusterID, mapping.DestinationNamespace, mapping.DestinationName)
		} else {
			return fmt.Errorf("%w: unsupported dependency kind %q", ErrInvalidRequest, mapping.Kind)
		}
		if err != nil {
			return fmt.Errorf("%w: %s %s/%s lookup failed: %v", ErrDestinationUnavailable, mapping.Kind, mapping.DestinationNamespace, mapping.DestinationName, err)
		}
		if !exists {
			return fmt.Errorf("%w: %s %s/%s is missing on the destination cluster", ErrDependencyUnresolved, mapping.Kind, mapping.DestinationNamespace, mapping.DestinationName)
		}
	}
	for _, ref := range refs {
		if !providedByKey[dependencyKey(ref.Kind, ref.SourceNamespace, ref.SourceName)] {
			return fmt.Errorf("%w: %s %s/%s has no operator-provided mapping", ErrDependencyUnresolved, ref.Kind, ref.SourceNamespace, ref.SourceName)
		}
	}
	return nil
}

func validateRequest(request PreviewRequest) error {
	if request.SourceClusterID < 1 || request.DestinationClusterID < 1 {
		return ErrInvalidRequest
	}
	if request.SourceClusterID == request.DestinationClusterID {
		return ErrInvalidRequest
	}
	if !validName(request.SourceNamespace, 63) || !validName(request.DestinationNamespace, 63) {
		return ErrInvalidRequest
	}
	if len(request.Bundle) == 0 {
		return ErrBundleEmpty
	}
	seen := make(map[string]bool, len(request.Bundle))
	for _, item := range request.Bundle {
		if !validPromotionKind(item.Kind) {
			return ErrInvalidRequest
		}
		if !validName(item.Namespace, 63) || !validName(item.Name, 253) {
			return ErrInvalidRequest
		}
		if item.Namespace != request.SourceNamespace {
			return ErrInvalidRequest
		}
		key := item.Kind + "/" + item.Namespace + "/" + item.Name
		if seen[key] {
			return ErrInvalidRequest
		}
		seen[key] = true
	}
	seenMappings := make(map[string]bool, len(request.DependencyMappings))
	for _, mapping := range request.DependencyMappings {
		if mapping.Kind != KindConfigMap && mapping.Kind != KindSecret {
			return ErrInvalidRequest
		}
		if !validName(mapping.SourceNamespace, 63) || !validName(mapping.SourceName, 253) {
			return ErrInvalidRequest
		}
		if !validName(mapping.DestinationNamespace, 63) || !validName(mapping.DestinationName, 253) {
			return ErrInvalidRequest
		}
		if mapping.SourceNamespace != request.SourceNamespace || mapping.DestinationNamespace != request.DestinationNamespace {
			return ErrInvalidRequest
		}
		key := dependencyKey(mapping.Kind, mapping.SourceNamespace, mapping.SourceName)
		if seenMappings[key] {
			return ErrInvalidRequest
		}
		seenMappings[key] = true
	}
	return nil
}

func validName(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && kubernetesNamePattern.MatchString(value)
}

func validPromotionKind(kind string) bool {
	return kind == KindDeployment || kind == KindService || kind == KindIngress
}

// stripRuntimeFields decodes the raw manifest, drops all server-owned and
// runtime fields, and re-encodes it. It returns the stripped manifest plus the
// source UID and resourceVersion (captured before stripping) so the plan can
// record the source evidence.
func stripRuntimeFields(raw json.RawMessage) (JSON, string, string, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, "", "", fmt.Errorf("decode manifest: %w", err)
	}
	metadata, _ := obj["metadata"].(map[string]any)
	var sourceUID, sourceRV string
	if metadata != nil {
		sourceUID, _ = metadata["uid"].(string)
		sourceRV, _ = metadata["resourceVersion"].(string)
	}
	obj = stripMap(obj, runtimeFieldNames())
	if metadata, ok := obj["metadata"].(map[string]any); ok {
		for _, field := range metadataRuntimeFields() {
			delete(metadata, field)
		}
		obj["metadata"] = metadata
	}
	if kind, _ := obj["kind"].(string); kind == KindService {
		stripServiceAllocatedFields(obj)
	}
	stripped, err := json.Marshal(obj)
	if err != nil {
		return nil, "", "", fmt.Errorf("encode stripped manifest: %w", err)
	}
	return JSON(stripped), sourceUID, sourceRV, nil
}

func stripServiceAllocatedFields(obj map[string]any) {
	spec, _ := obj["spec"].(map[string]any)
	if spec == nil {
		return
	}
	for _, field := range []string{"clusterIP", "clusterIPs", "ipFamilies", "ipFamilyPolicy", "healthCheckNodePort"} {
		delete(spec, field)
	}
	if ports, ok := spec["ports"].([]any); ok {
		for _, entry := range ports {
			if port, ok := entry.(map[string]any); ok {
				delete(port, "nodePort")
			}
		}
	}
}

func runtimeFieldNames() []string {
	return []string{"status"}
}

func metadataRuntimeFields() []string {
	return []string{
		"uid", "resourceVersion", "creationTimestamp", "generation",
		"managedFields", "selfLink", "ownerReferences", "resourceVersion",
		"finalizers", "deletionTimestamp", "deletionGracePeriodSeconds",
	}
}

func stripMap(obj map[string]any, drop []string) map[string]any {
	cleaned := make(map[string]any, len(obj))
	for key, value := range obj {
		if shouldDrop(key, drop) {
			continue
		}
		cleaned[key] = stripValue(value)
	}
	return cleaned
}

func stripValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return stripMap(typed, nil)
	case []any:
		cleaned := make([]any, 0, len(typed))
		for _, item := range typed {
			cleaned = append(cleaned, stripValue(item))
		}
		return cleaned
	}
	return value
}

func shouldDrop(key string, drop []string) bool {
	for _, name := range drop {
		if key == name {
			return true
		}
	}
	return false
}

// rewriteNamespace rewrites the metadata.namespace of the stripped manifest to
// the destination namespace. It does not rewrite selectors or ingress
// backends; the operator is responsible for namespace-scoped consistency.
func rewriteNamespace(manifest JSON, destinationNamespace string) (JSON, error) {
	var obj map[string]any
	if err := json.Unmarshal(manifest, &obj); err != nil {
		return nil, err
	}
	metadata, _ := obj["metadata"].(map[string]any)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["namespace"] = destinationNamespace
	obj["metadata"] = metadata
	return json.Marshal(obj)
}

// DependencyReference is a ConfigMap/Secret reference discovered in the
// stripped manifest, scoped to the source namespace. Volumes and envFrom
// references without an explicit namespace are treated as source-namespace
// scoped.
type DependencyReference struct {
	Kind            string
	SourceNamespace string
	SourceName      string
}

// scanDependencies walks a stripped Deployment manifest and collects every
// ConfigMap and Secret reference it can find. Only Deployment manifests carry
// pod templates; Service and Ingress manifests return no references.
func scanDependencies(manifest JSON, sourceNamespace string) []DependencyReference {
	var obj map[string]any
	if err := json.Unmarshal(manifest, &obj); err != nil {
		return nil
	}
	refs := []DependencyReference{}
	kind, _ := obj["kind"].(string)
	if kind != KindDeployment {
		return refs
	}
	podTemplate := walkPath(obj, "spec", "template", "spec")
	if podTemplate == nil {
		return refs
	}
	if containers, ok := podTemplate["containers"].([]any); ok {
		for _, container := range containers {
			if containerMap, ok := container.(map[string]any); ok {
				refs = append(refs, scanContainerRefs(containerMap, sourceNamespace)...)
			}
		}
	}
	if initContainers, ok := podTemplate["initContainers"].([]any); ok {
		for _, container := range initContainers {
			if containerMap, ok := container.(map[string]any); ok {
				refs = append(refs, scanContainerRefs(containerMap, sourceNamespace)...)
			}
		}
	}
	if volumes, ok := podTemplate["volumes"].([]any); ok {
		for _, volume := range volumes {
			if volumeMap, ok := volume.(map[string]any); ok {
				refs = append(refs, scanVolumeRefs(volumeMap, sourceNamespace)...)
			}
		}
	}
	if imagePullSecrets, ok := podTemplate["imagePullSecrets"].([]any); ok {
		for _, entry := range imagePullSecrets {
			if entryMap, ok := entry.(map[string]any); ok {
				if name, ok := entryMap["name"].(string); ok && name != "" {
					refs = append(refs, DependencyReference{Kind: KindSecret, SourceNamespace: sourceNamespace, SourceName: name})
				}
			}
		}
	}
	return dedupeRefs(refs)
}

func rewriteDependencyMappings(manifest JSON, mappings []DependencyMapping, sourceNamespace string) (JSON, error) {
	destinationNames := make(map[string]string, len(mappings))
	for _, mapping := range mappings {
		destinationNames[dependencyKey(mapping.Kind, mapping.SourceNamespace, mapping.SourceName)] = mapping.DestinationName
	}
	if len(destinationNames) == 0 {
		return manifest, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(manifest, &obj); err != nil {
		return nil, fmt.Errorf("decode dependency manifest: %w", err)
	}
	podSpec := walkPath(obj, "spec", "template", "spec")
	if podSpec == nil {
		return manifest, nil
	}
	rewriteName := func(kind string, reference map[string]any, field string) {
		sourceName, _ := reference[field].(string)
		if destinationName := destinationNames[dependencyKey(kind, sourceNamespace, sourceName)]; destinationName != "" {
			reference[field] = destinationName
		}
	}
	rewriteContainers := func(value any) {
		containers, _ := value.([]any)
		for _, entry := range containers {
			container, _ := entry.(map[string]any)
			if container == nil {
				continue
			}
			if envFrom, ok := container["envFrom"].([]any); ok {
				for _, raw := range envFrom {
					ref, _ := raw.(map[string]any)
					if configMapRef, ok := ref["configMapRef"].(map[string]any); ok {
						rewriteName(KindConfigMap, configMapRef, "name")
					}
					if secretRef, ok := ref["secretRef"].(map[string]any); ok {
						rewriteName(KindSecret, secretRef, "name")
					}
				}
			}
			if env, ok := container["env"].([]any); ok {
				for _, raw := range env {
					variable, _ := raw.(map[string]any)
					valueFrom, _ := variable["valueFrom"].(map[string]any)
					if configMapRef, ok := valueFrom["configMapKeyRef"].(map[string]any); ok {
						rewriteName(KindConfigMap, configMapRef, "name")
					}
					if secretRef, ok := valueFrom["secretKeyRef"].(map[string]any); ok {
						rewriteName(KindSecret, secretRef, "name")
					}
				}
			}
		}
	}
	rewriteContainers(podSpec["containers"])
	rewriteContainers(podSpec["initContainers"])
	if volumes, ok := podSpec["volumes"].([]any); ok {
		for _, raw := range volumes {
			volume, _ := raw.(map[string]any)
			if configMap, ok := volume["configMap"].(map[string]any); ok {
				rewriteName(KindConfigMap, configMap, "name")
			}
			if secret, ok := volume["secret"].(map[string]any); ok {
				rewriteName(KindSecret, secret, "secretName")
			}
			if projected, ok := volume["projected"].(map[string]any); ok {
				if sources, ok := projected["sources"].([]any); ok {
					for _, sourceRaw := range sources {
						source, _ := sourceRaw.(map[string]any)
						if configMap, ok := source["configMap"].(map[string]any); ok {
							rewriteName(KindConfigMap, configMap, "name")
						}
						if secret, ok := source["secret"].(map[string]any); ok {
							rewriteName(KindSecret, secret, "name")
						}
					}
				}
			}
		}
	}
	if imagePullSecrets, ok := podSpec["imagePullSecrets"].([]any); ok {
		for _, raw := range imagePullSecrets {
			if secret, ok := raw.(map[string]any); ok {
				rewriteName(KindSecret, secret, "name")
			}
		}
	}
	rewritten, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("encode dependency manifest: %w", err)
	}
	return rewritten, nil
}

func scanContainerRefs(container map[string]any, sourceNamespace string) []DependencyReference {
	refs := []DependencyReference{}
	if envFrom, ok := container["envFrom"].([]any); ok {
		for _, entry := range envFrom {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if cmRef, ok := entryMap["configMapRef"].(map[string]any); ok {
				refs = append(refs, refFromObjectReference(cmRef, KindConfigMap, sourceNamespace))
			}
			if secRef, ok := entryMap["secretRef"].(map[string]any); ok {
				refs = append(refs, refFromObjectReference(secRef, KindSecret, sourceNamespace))
			}
		}
	}
	if env, ok := container["env"].([]any); ok {
		for _, entry := range env {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			valueFrom, _ := entryMap["valueFrom"].(map[string]any)
			if valueFrom == nil {
				continue
			}
			if cmRef, ok := valueFrom["configMapKeyRef"].(map[string]any); ok {
				refs = append(refs, refFromObjectReference(cmRef, KindConfigMap, sourceNamespace))
			}
			if secRef, ok := valueFrom["secretKeyRef"].(map[string]any); ok {
				refs = append(refs, refFromObjectReference(secRef, KindSecret, sourceNamespace))
			}
		}
	}
	return refs
}

func scanVolumeRefs(volume map[string]any, sourceNamespace string) []DependencyReference {
	refs := []DependencyReference{}
	if cm, ok := volume["configMap"].(map[string]any); ok {
		refs = append(refs, refFromObjectReference(cm, KindConfigMap, sourceNamespace))
	}
	if secret, ok := volume["secret"].(map[string]any); ok {
		if name, ok := secret["secretName"].(string); ok && name != "" {
			refs = append(refs, DependencyReference{Kind: KindSecret, SourceNamespace: sourceNamespace, SourceName: name})
		}
	}
	if projected, ok := volume["projected"].(map[string]any); ok {
		if sources, ok := projected["sources"].([]any); ok {
			for _, source := range sources {
				sourceMap, ok := source.(map[string]any)
				if !ok {
					continue
				}
				if cmRef, ok := sourceMap["configMap"].(map[string]any); ok {
					refs = append(refs, refFromObjectReference(cmRef, KindConfigMap, sourceNamespace))
				}
				if secRef, ok := sourceMap["secret"].(map[string]any); ok {
					refs = append(refs, refFromObjectReference(secRef, KindSecret, sourceNamespace))
				}
			}
		}
	}
	return refs
}

func refFromObjectReference(ref map[string]any, kind, defaultNamespace string) DependencyReference {
	name, _ := ref["name"].(string)
	if name == "" {
		return DependencyReference{}
	}
	namespace, _ := ref["namespace"].(string)
	if namespace == "" {
		namespace = defaultNamespace
	}
	return DependencyReference{Kind: kind, SourceNamespace: namespace, SourceName: name}
}

func dedupeRefs(refs []DependencyReference) []DependencyReference {
	seen := make(map[string]bool, len(refs))
	cleaned := refs[:0]
	for _, ref := range refs {
		if ref.SourceName == "" {
			continue
		}
		key := dependencyKey(ref.Kind, ref.SourceNamespace, ref.SourceName)
		if seen[key] {
			continue
		}
		seen[key] = true
		cleaned = append(cleaned, ref)
	}
	return cleaned
}

func buildDependencyRecords(request PreviewRequest, refs []DependencyReference) []DependencyRecord {
	records := make([]DependencyRecord, 0, len(request.DependencyMappings))
	refSet := make(map[string]bool, len(refs))
	for _, ref := range refs {
		refSet[dependencyKey(ref.Kind, ref.SourceNamespace, ref.SourceName)] = true
	}
	for _, mapping := range request.DependencyMappings {
		resolved := refSet[dependencyKey(mapping.Kind, mapping.SourceNamespace, mapping.SourceName)]
		records = append(records, DependencyRecord{
			Kind:                 mapping.Kind,
			SourceNamespace:      mapping.SourceNamespace,
			SourceName:           mapping.SourceName,
			DestinationNamespace: mapping.DestinationNamespace,
			DestinationName:      mapping.DestinationName,
			Resolved:             resolved,
		})
	}
	return records
}

func buildDiff(kind string, manifest JSON) ItemDiff {
	var obj map[string]any
	if err := json.Unmarshal(manifest, &obj); err != nil {
		return ItemDiff{Mode: ModeCreate}
	}
	after := map[string]any{}
	if metadata, ok := obj["metadata"].(map[string]any); ok {
		if name, ok := metadata["name"].(string); ok {
			after["name"] = name
		}
		if ns, ok := metadata["namespace"].(string); ok {
			after["namespace"] = ns
		}
		if labels, ok := metadata["labels"].(map[string]any); ok {
			after["labels"] = labels
		}
	}
	switch kind {
	case KindDeployment:
		if spec, ok := obj["spec"].(map[string]any); ok {
			if replicas, ok := spec["replicas"]; ok {
				after["replicas"] = replicas
			}
			if template, ok := spec["template"].(map[string]any); ok {
				if templateSpec, ok := template["spec"].(map[string]any); ok {
					if containers, ok := templateSpec["containers"].([]any); ok {
						images := []string{}
						for _, container := range containers {
							if containerMap, ok := container.(map[string]any); ok {
								if name, ok := containerMap["name"].(string); ok {
									if image, ok := containerMap["image"].(string); ok {
										images = append(images, name+"="+image)
									}
								}
							}
						}
						after["images"] = images
					}
				}
			}
		}
	case KindService:
		if spec, ok := obj["spec"].(map[string]any); ok {
			if t, ok := spec["type"].(string); ok {
				after["type"] = t
			}
			if ports, ok := spec["ports"].([]any); ok {
				after["ports"] = ports
			}
		}
	case KindIngress:
		if spec, ok := obj["spec"].(map[string]any); ok {
			if rules, ok := spec["rules"].([]any); ok {
				after["rules"] = rules
			}
		}
	}
	return ItemDiff{Mode: ModeCreate, After: after, ChangedFields: []string{"create"}}
}

func buildBundleSummary(items []BundleItem) BundleSummary {
	summary := BundleSummary{ItemCount: len(items)}
	for _, item := range items {
		switch item.Kind {
		case KindDeployment:
			summary.DeploymentCount++
		case KindService:
			summary.ServiceCount++
		case KindIngress:
			summary.IngressCount++
		}
		if item.ItemStatus == ItemStatusPending {
			summary.PendingCount++
		}
	}
	return summary
}

func walkPath(obj map[string]any, keys ...string) map[string]any {
	current := obj
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func promotionPathForExists(kind, namespace, name string) (string, error) {
	return promotionPathLookup(kind, namespace, name)
}

func promotionPathLookup(kind, namespace, name string) (string, error) {
	switch kind {
	case KindDeployment:
		return "/apis/apps/v1/namespaces/" + namespace + "/deployments/" + name, nil
	case KindService:
		return "/api/v1/namespaces/" + namespace + "/services/" + name, nil
	case KindIngress:
		return "/apis/networking.k8s.io/v1/namespaces/" + namespace + "/ingresses/" + name, nil
	}
	return "", fmt.Errorf("unsupported promotion kind %q", kind)
}

func promotionCreatePath(kind, namespace string) (string, error) {
	switch kind {
	case KindDeployment:
		return "/apis/apps/v1/namespaces/" + namespace + "/deployments", nil
	case KindService:
		return "/api/v1/namespaces/" + namespace + "/services", nil
	case KindIngress:
		return "/apis/networking.k8s.io/v1/namespaces/" + namespace + "/ingresses", nil
	}
	return "", fmt.Errorf("unsupported promotion kind %q", kind)
}

func dependencyKey(kind, namespace, name string) string {
	return kind + "/" + namespace + "/" + name
}

func safeExecutionError(err error) string {
	var status interface{ StatusCode() int }
	if errors.As(err, &status) {
		return fmt.Sprintf("Kubernetes API rejected promotion with HTTP %d", status.StatusCode())
	}
	text := err.Error()
	if len(text) > 480 {
		text = text[:480]
	}
	return text
}

func truncate(text string) string {
	if len(text) > 480 {
		return text[:480]
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
