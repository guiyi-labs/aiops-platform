package correlation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

// SignalOccurrenceInput is the typed input from the signal package. The
// correlation package stays independent of signal.Occurrence so the engine
// can be tested with fixtures and replayed deterministically.
type SignalOccurrenceInput struct {
	ID          int64
	SignalID    string
	Producer    string
	ClusterID   int64
	Namespace   string
	Resource    ResourceCitation
	Severity    string
	State       string
	Coverage    string
	Freshness   time.Time
	WindowStart *time.Time
	WindowEnd   *time.Time
	ObservedAt  time.Time
	Evidence    []EvidenceRef
}

// ChangeEventInput is the typed input from the topology package's ChangeEvent.
type ChangeEventInput struct {
	ID         int64
	ClusterID  int64
	Namespace  string
	Kind       string // promotion | backup | maintenance | restore | rollout | audit
	PlanID     string
	Target     ResourceCitation
	Action     string
	Result     string // succeeded | failed | pending | partial
	Actor      string
	StartedAt  time.Time
	FinishedAt *time.Time
	Evidence   []EvidenceRef
	Confidence string // high | low
	Source     string // platform | k8s_event | delivery_adapter
}

// TopologyEdgeInput is the typed input from the topology package's Edge.
type TopologyEdgeInput struct {
	ID        int64
	ClusterID int64
	Kind      string // EdgeKind string value: Owns | Selects | RoutesTo | BackedBy | RunsOn | Mounts | Scales | ProtectedBy
	Source    ResourceCitation
	Target    ResourceCitation
	ValidFrom time.Time
	ValidTo   *time.Time
}

// DiagnosisRef is the typed input from the diagnosis package's Record. Only
// the fields needed for correlation are included.
type DiagnosisRef struct {
	ID         int64
	ClusterID  int64
	RuleID     string
	Resource   ResourceCitation
	Severity   string
	Status     string
	ObservedAt time.Time
}

// EngineInputs is the full input set for one correlation pass. The caller
// (service layer) gathers these via the signal, topology and diagnosis
// repositories and maps them into the typed inputs. The engine is pure with
// respect to these inputs: identical inputs + identical rule/correlation
// versions yield identical cases.
type EngineInputs struct {
	Signals   []SignalOccurrenceInput
	Changes   []ChangeEventInput
	Edges     []TopologyEdgeInput
	Diagnoses []DiagnosisRef
	Now       time.Time
}

// Engine is the deterministic correlation engine. It is stateless; all state
// lives in the repository. The engine is the only producer of Case structs;
// the service is the only writer to the repository.
type Engine struct{}

// NewEngine constructs an Engine.
func NewEngine() *Engine { return &Engine{} }

// Correlate runs the deterministic correlation rules over the inputs and
// returns the results produced. Each trigger signal is evaluated against all
// matching rules; a single signal may produce multiple results (e.g. a pod
// failure may trigger both a rollout rule and a metric rule).
//
// The engine is pure: it does not read from or write to the repository. The
// service layer persists the returned results. Identical inputs + identical
// rule/correlation versions yield identical results.
func (e *Engine) Correlate(ctx context.Context, inputs EngineInputs) ([]CorrelationResult, error) {
	if inputs.Now.IsZero() {
		inputs.Now = time.Now().UTC()
	}

	// Build a topology adjacency index for same-cluster path queries.
	edgeIndex := buildEdgeIndex(inputs.Edges)

	// Build a change index by cluster+kind for quick lookup.
	changeIndex := buildChangeIndex(inputs.Changes)

	// Build a diagnosis index by cluster+resource for match factor.
	diagIndex := buildDiagnosisIndex(inputs.Diagnoses)

	var results []CorrelationResult
	seenCaseKeys := make(map[string]int) // case_key → index in results

	// Process trigger signals in stable order (by observed_at, then signal_id).
	triggers := filterTriggerSignals(inputs.Signals)
	sortTriggerSignals(triggers)

	for _, sig := range triggers {
		rules := RulesForTriggerSignal(sig.SignalID)
		for _, rule := range rules {
			result, ok := e.evaluateRule(ctx, rule, sig, inputs, edgeIndex, changeIndex, diagIndex)
			if !ok {
				continue
			}
			if idx, exists := seenCaseKeys[result.Case.CaseKey]; exists {
				// Merge: the same case_key was already produced by a previous
				// trigger signal. Merge factors, signal links and change
				// candidates; keep the widest observed window.
				existing := &results[idx]
				existing.Case.Factors = mergeFactors(existing.Case.Factors, result.Case.Factors)
				if result.Case.FirstObservedAt.Before(existing.Case.FirstObservedAt) {
					existing.Case.FirstObservedAt = result.Case.FirstObservedAt
				}
				if result.Case.LastObservedAt.After(existing.Case.LastObservedAt) {
					existing.Case.LastObservedAt = result.Case.LastObservedAt
				}
				// Promote confidence if the new result is better.
				if confidenceRank(result.Case.Confidence) < confidenceRank(existing.Case.Confidence) {
					existing.Case.Confidence = result.Case.Confidence
				}
				existing.ChangeCandidates = append(existing.ChangeCandidates, result.ChangeCandidates...)
				existing.SignalLinks = append(existing.SignalLinks, result.SignalLinks...)
				existing.ResourceLinks = append(existing.ResourceLinks, result.ResourceLinks...)
				continue
			}
			seenCaseKeys[result.Case.CaseKey] = len(results)
			results = append(results, result)
		}
	}

	// Stable sort results by first_observed_at for deterministic output.
	sortResultsByFirstObserved(results)
	return results, nil
}

// evaluateRule evaluates one rule against one trigger signal. Returns ok=false
// when the rule does not match (e.g. no change event in the time window, or
// the signal's resource kind does not match the rule's primary kind).
func (e *Engine) evaluateRule(
	_ context.Context,
	rule RuleDescriptor,
	sig SignalOccurrenceInput,
	inputs EngineInputs,
	edgeIndex *edgeIndex,
	changeIndex *changeIndex,
	diagIndex *diagIndex,
) (CorrelationResult, bool) {
	// The signal's resource must match the rule's primary kind (or be a
	// topology neighbor of it). For now we check the direct kind match.
	if sig.Resource.Kind != rule.PrimaryKind {
		// Allow Service→Pod topology: if the signal is on a Pod but the rule
		// primary is Service, check if there's a Service that backs the Pod.
		if rule.PrimaryKind == "Service" && sig.Resource.Kind == "Pod" {
			// ok — the engine will compute the topology path
		} else if rule.PrimaryKind == "Pod" && sig.Resource.Kind == "Node" {
			// metric breach on Node correlates with pod rollout via RunsOn
		} else {
			return CorrelationResult{}, false
		}
	}

	// Find change events in the time window that match the rule's change kinds.
	windowStart := sig.ObservedAt.Add(-time.Duration(rule.TimeWindowSecs) * time.Second)
	candidates := changeIndex.lookup(sig.ClusterID, rule.ChangeKinds, windowStart, sig.ObservedAt)
	if len(candidates) == 0 {
		// No change in the window — this is a cold-start case. Still produce
		// a case with ConfidenceUnknown so M43 can disclose uncertainty.
		return e.buildColdStartResult(rule, sig, inputs.Now), true
	}

	// Evaluate factors for each candidate change.
	var changeCandidates []ChangeCandidate
	var allFactors []Factor
	var bestConfidence ConfidenceClass = ConfidenceUnknown
	var topCandidateID *int64

	for i, change := range candidates {
		factors, contradicting := e.computeFactors(rule, sig, change, edgeIndex, diagIndex, inputs.Now)
		confidence := classifyConfidence(rule, factors, contradicting)
		reasonCode := computeReasonCode(rule, confidence, change)

		rank := i + 1
		cc := ChangeCandidate{
			CaseID:            0, // set by repository on persist
			ChangeEventID:     change.ID,
			RuleID:            rule.RuleID,
			Confidence:        confidence,
			Rank:              rank,
			Factors:           factors,
			EvidenceRefs:      change.Evidence,
			ContradictingRefs: contradicting,
			ReasonCode:        reasonCode,
			CreatedAt:         inputs.Now,
			UpdatedAt:         inputs.Now,
		}
		changeCandidates = append(changeCandidates, cc)

		// Track the best confidence and top candidate.
		if confidenceRank(confidence) < confidenceRank(bestConfidence) {
			bestConfidence = confidence
			if confidence == ConfidenceConfirmed {
				// ID will be set by repository; use a placeholder pointer.
				tmp := int64(0)
				topCandidateID = &tmp
			}
		}
		allFactors = mergeFactors(allFactors, factors)
	}

	// Build the case.
	caseKey := computeCaseKey(sig.ClusterID, sig.Resource, rule.RuleID)
	primaryResource := sig.Resource
	if primaryResource.UID == "" {
		primaryResource.Incomplete = true
	}

	completeness := classifyCompleteness(allFactors, changeCandidates)
	firstObserved := sig.ObservedAt
	lastObserved := sig.ObservedAt

	c := Case{
		CaseKey:               caseKey,
		ClusterID:             sig.ClusterID,
		RuleID:                rule.RuleID,
		CorrelationVersion:    CorrelationVersion,
		PrimaryResource:       primaryResource,
		Status:                CaseStatusActive,
		Confidence:            bestConfidence,
		EvidenceCompleteness:  completeness,
		Factors:               allFactors,
		FirstObservedAt:       firstObserved,
		LastObservedAt:        lastObserved,
		RootChangeCandidateID: topCandidateID,
		CreatedAt:             inputs.Now,
		UpdatedAt:             inputs.Now,
	}

	return CorrelationResult{
		Case:             c,
		ChangeCandidates: changeCandidates,
		SignalLinks:      []SignalLink{buildTriggerLink(sig, inputs.Now)},
		ResourceLinks:    []ResourceLink{buildPrimaryLink(primaryResource, inputs.Now)},
	}, true
}

// buildTriggerLink constructs the trigger signal link for the case.
func buildTriggerLink(sig SignalOccurrenceInput, now time.Time) SignalLink {
	return SignalLink{
		SignalOccurrenceID: sig.ID,
		Relation:           SignalRelationTrigger,
		SignalID:           sig.SignalID,
		Producer:           sig.Producer,
		ObservedAt:         sig.ObservedAt,
		CreatedAt:          now,
	}
}

// buildPrimaryLink constructs the primary resource link for the case.
func buildPrimaryLink(primary ResourceCitation, now time.Time) ResourceLink {
	return ResourceLink{
		Resource:  primary,
		Relation:  ResourceRelationPrimary,
		CreatedAt: now,
	}
}

// buildColdStartResult produces a case with ConfidenceUnknown when no change
// event is found in the time window. The case is retained so M43 can disclose
// uncertainty; no root cause is asserted.
func (e *Engine) buildColdStartResult(rule RuleDescriptor, sig SignalOccurrenceInput, now time.Time) CorrelationResult {
	caseKey := computeCaseKey(sig.ClusterID, sig.Resource, rule.RuleID)
	primary := sig.Resource
	if primary.UID == "" {
		primary.Incomplete = true
	}
	c := Case{
		CaseKey:              caseKey,
		ClusterID:            sig.ClusterID,
		RuleID:               rule.RuleID,
		CorrelationVersion:   CorrelationVersion,
		PrimaryResource:      primary,
		Status:               CaseStatusActive,
		Confidence:           ConfidenceUnknown,
		EvidenceCompleteness: CompletenessInsufficient,
		Factors: []Factor{
			{Kind: "signal_freshness", Value: "no_change_in_window", Weight: 0.0},
		},
		FirstObservedAt: sig.ObservedAt,
		LastObservedAt:  sig.ObservedAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	return CorrelationResult{
		Case:          c,
		SignalLinks:   []SignalLink{buildTriggerLink(sig, now)},
		ResourceLinks: []ResourceLink{buildPrimaryLink(primary, now)},
	}
}

// computeFactors evaluates the deterministic factors for one (signal, change)
// pair. Returns the matching factors and any contradicting evidence refs.
func (e *Engine) computeFactors(
	rule RuleDescriptor,
	sig SignalOccurrenceInput,
	change ChangeEventInput,
	edgeIndex *edgeIndex,
	diagIndex *diagIndex,
	now time.Time,
) (factors []Factor, contradicting []EvidenceRef) {
	// Factor: same_uid — does the signal's resource UID match the change's
	// target UID?
	if sig.Resource.UID != "" && sig.Resource.UID == change.Target.UID {
		factors = append(factors, Factor{
			Kind:   "same_uid",
			Value:  "match",
			Weight: 1.0,
			Evidence: []EvidenceRef{
				{Kind: "signal_occurrence", ID: sig.ID},
				{Kind: "change_event", ID: change.ID},
			},
		})
	} else if sig.Resource.UID != "" && change.Target.UID != "" && sig.Resource.UID != change.Target.UID {
		// Different UIDs — check topology distance instead.
		factors = append(factors, Factor{
			Kind:   "same_uid",
			Value:  "mismatch",
			Weight: 0.0,
		})
	}

	// Factor: topology_distance — is there a topology path from the signal's
	// resource to the change's target?
	path, edgeIDs := edgeIndex.findPath(sig.ClusterID, sig.Resource, change.Target, rule.PrimaryKind)
	if len(path) > 0 {
		factors = append(factors, Factor{
			Kind:   "topology_distance",
			Value:  fmt.Sprintf("%d", len(path)),
			Weight: 1.0 / float64(len(path)),
			Evidence: func() []EvidenceRef {
				refs := make([]EvidenceRef, 0, len(edgeIDs))
				for _, eid := range edgeIDs {
					refs = append(refs, EvidenceRef{Kind: "topology_edge", ID: eid})
				}
				return refs
			}(),
		})
	} else if sig.Resource.UID != change.Target.UID {
		// No topology path and different UIDs — unrelated topology.
		factors = append(factors, Factor{
			Kind:   "topology_distance",
			Value:  "unreachable",
			Weight: 0.0,
		})
	}

	// Factor: time_distance — did the change start within the rule's time
	// window before the symptom?
	timeDelta := sig.ObservedAt.Sub(change.StartedAt)
	if timeDelta >= 0 && timeDelta <= time.Duration(rule.TimeWindowSecs)*time.Second {
		factors = append(factors, Factor{
			Kind:   "time_distance",
			Value:  fmt.Sprintf("%.0fs", timeDelta.Seconds()),
			Weight: 1.0 - (timeDelta.Seconds() / float64(rule.TimeWindowSecs)),
			Evidence: []EvidenceRef{
				{Kind: "change_event", ID: change.ID},
				{Kind: "signal_occurrence", ID: sig.ID},
			},
		})
	} else {
		// Change started after the symptom or outside the window — not causal.
		factors = append(factors, Factor{
			Kind:   "time_distance",
			Value:  "out_of_window",
			Weight: 0.0,
		})
	}

	// Factor: change_symptom_rule — does the change kind match the rule's
	// change kinds?
	for _, ck := range rule.ChangeKinds {
		if ck == change.Kind {
			factors = append(factors, Factor{
				Kind:   "change_symptom_rule",
				Value:  "match",
				Weight: 1.0,
				Evidence: []EvidenceRef{
					{Kind: "change_event", ID: change.ID},
				},
			})
			break
		}
	}

	// Factor: signal_freshness — is the signal fresh enough?
	if !sig.Freshness.IsZero() {
		freshnessDelta := now.Sub(sig.Freshness)
		if freshnessDelta <= 5*time.Minute {
			factors = append(factors, Factor{
				Kind:   "signal_freshness",
				Value:  "fresh",
				Weight: 1.0,
			})
		} else if freshnessDelta <= 1*time.Hour {
			factors = append(factors, Factor{
				Kind:   "signal_freshness",
				Value:  "stale",
				Weight: 0.5,
			})
		} else {
			factors = append(factors, Factor{
				Kind:   "signal_freshness",
				Value:  "very_stale",
				Weight: 0.0,
			})
		}
	}

	// Factor: signal_completeness — is the signal coverage complete?
	switch sig.Coverage {
	case "complete":
		factors = append(factors, Factor{Kind: "signal_completeness", Value: "complete", Weight: 1.0})
	case "partial":
		factors = append(factors, Factor{Kind: "signal_completeness", Value: "partial", Weight: 0.5})
	default:
		factors = append(factors, Factor{Kind: "signal_completeness", Value: sig.Coverage, Weight: 0.0})
	}

	// Factor: diagnosis_match — is there a matching diagnosis record?
	if diags := diagIndex.lookup(sig.ClusterID, sig.Resource); len(diags) > 0 {
		refs := make([]EvidenceRef, 0, len(diags))
		for _, d := range diags {
			refs = append(refs, EvidenceRef{Kind: "diagnosis_record", ID: d.ID})
		}
		factors = append(factors, Factor{
			Kind:     "diagnosis_match",
			Value:    "match",
			Weight:   1.0,
			Evidence: refs,
		})
	}

	// Contradicting factor: the change's target is a different UID with no
	// topology path to the signal's resource. This was already recorded as a
	// "topology_distance: unreachable" factor above; here we add the
	// contradicting evidence ref so classifyConfidence can downgrade.
	//
	// Note: "change succeeded + signal active" is NOT contradicting for cause
	// correlation. A rollout that "succeeded" (completed) and then caused a
	// pod failure is the expected pattern — "succeeded" means the rollout
	// action completed, not that it fixed a problem. Contradiction applies to
	// remediation evaluation, not cause correlation.
	if sig.Resource.UID != "" && change.Target.UID != "" && sig.Resource.UID != change.Target.UID {
		if _, edgeIDs := edgeIndex.findPath(sig.ClusterID, sig.Resource, change.Target, rule.PrimaryKind); len(edgeIDs) == 0 {
			contradicting = append(contradicting, EvidenceRef{
				Kind: "change_event",
				ID:   change.ID,
			})
		}
	}

	return factors, contradicting
}

// classifyConfidence determines the confidence class from the factors and
// contradicting evidence. The rule's RequiredFactors must all be present and
// no contradicting factor may exist for ConfidenceConfirmed.
func classifyConfidence(rule RuleDescriptor, factors []Factor, contradicting []EvidenceRef) ConfidenceClass {
	if len(contradicting) > 0 {
		// Contradicting evidence present — downgrade to candidate at best.
		// If all required factors also match, it's still a candidate (not
		// confirmed). If required factors are missing, it's contradicted.
		if hasAllRequiredFactors(rule, factors) {
			return ConfidenceCandidate
		}
		return ConfidenceContradicted
	}

	if !hasAllRequiredFactors(rule, factors) {
		// Missing required factors — check how many are present.
		present := countRequiredFactors(rule, factors)
		if present == 0 {
			return ConfidenceUnknown
		}
		return ConfidenceCandidate
	}

	// All required factors present, no contradicting evidence — confirmed.
	return ConfidenceConfirmed
}

// hasAllRequiredFactors returns true when all RequiredFactors in the rule
// have a matching factor with weight > 0.
func hasAllRequiredFactors(rule RuleDescriptor, factors []Factor) bool {
	for _, req := range rule.RequiredFactors {
		found := false
		for _, f := range factors {
			if f.Kind == req && f.Weight > 0 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// countRequiredFactors returns the number of required factors present with
// weight > 0.
func countRequiredFactors(rule RuleDescriptor, factors []Factor) int {
	count := 0
	for _, req := range rule.RequiredFactors {
		for _, f := range factors {
			if f.Kind == req && f.Weight > 0 {
				count++
				break
			}
		}
	}
	return count
}

// classifyCompleteness determines the evidence completeness from the factors
// and change candidates.
func classifyCompleteness(factors []Factor, candidates []ChangeCandidate) EvidenceCompleteness {
	if len(factors) == 0 {
		return CompletenessInsufficient
	}
	completeCount := 0
	for _, f := range factors {
		if f.Weight > 0.5 {
			completeCount++
		}
	}
	if completeCount >= len(factors)/2 && len(candidates) > 0 {
		return CompletenessComplete
	}
	if completeCount > 0 {
		return CompletenessPartial
	}
	return CompletenessInsufficient
}

// computeReasonCode returns the stable reason code for a change candidate.
func computeReasonCode(rule RuleDescriptor, confidence ConfidenceClass, change ChangeEventInput) string {
	switch confidence {
	case ConfidenceConfirmed:
		return rule.ReasonCode
	case ConfidenceCandidate:
		if change.Result == "succeeded" {
			return "change_succeeded_but_symptoms_persist"
		}
		return "partial_factor_match"
	case ConfidenceContradicted:
		return "contradicting_evidence"
	default:
		return "insufficient_evidence"
	}
}

// computeCaseKey computes the deterministic case key from the stable identity
// fields. case_key = SHA256(cluster_id | primary_uid | rule_id | correlation_version).
// Different UID, rule or version never merges.
func computeCaseKey(clusterID int64, resource ResourceCitation, ruleID string) string {
	uid := resource.UID
	if uid == "" {
		// Name-only fallback: use kind+namespace+name so name-only resources
		// still produce stable case keys. Marked incomplete in the case.
		uid = resource.Kind + "/" + resource.Namespace + "/" + resource.Name
	}
	data := fmt.Sprintf("%d|%s|%s|%s", clusterID, uid, ruleID, CorrelationVersion)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// confidenceRank returns a sortable rank where lower = better confidence.
func confidenceRank(c ConfidenceClass) int {
	switch c {
	case ConfidenceConfirmed:
		return 0
	case ConfidenceCandidate:
		return 1
	case ConfidenceContradicted:
		return 2
	default:
		return 3
	}
}

// filterTriggerSignals returns only signals that are trigger candidates for
// at least one rule. Non-trigger signals (e.g. alert.resolved, posture) are
// not used to open cases but may appear as context links.
func filterTriggerSignals(signals []SignalOccurrenceInput) []SignalOccurrenceInput {
	var out []SignalOccurrenceInput
	for _, sig := range signals {
		if len(RulesForTriggerSignal(sig.SignalID)) > 0 {
			out = append(out, sig)
		}
	}
	return out
}

// sortTriggerSignals sorts by observed_at, then signal_id, for deterministic
// case ordering.
func sortTriggerSignals(signals []SignalOccurrenceInput) {
	sort.SliceStable(signals, func(i, j int) bool {
		if !signals[i].ObservedAt.Equal(signals[j].ObservedAt) {
			return signals[i].ObservedAt.Before(signals[j].ObservedAt)
		}
		return signals[i].SignalID < signals[j].SignalID
	})
}

// sortResultsByFirstObserved sorts correlation results by the case's
// FirstObservedAt for deterministic output.
func sortResultsByFirstObserved(results []CorrelationResult) {
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Case.FirstObservedAt.Before(results[j].Case.FirstObservedAt)
	})
}

// sortCasesByFirstObserved sorts cases by FirstObservedAt for deterministic
// output.
func sortCasesByFirstObserved(cases []Case) {
	sort.SliceStable(cases, func(i, j int) bool {
		return cases[i].FirstObservedAt.Before(cases[j].FirstObservedAt)
	})
}

// mergeFactors merges two factor slices, keeping the highest weight per kind.
func mergeFactors(existing, new []Factor) []Factor {
	byKind := make(map[string]Factor)
	for _, f := range existing {
		byKind[f.Kind] = f
	}
	for _, f := range new {
		if cur, ok := byKind[f.Kind]; ok {
			if f.Weight > cur.Weight {
				byKind[f.Kind] = f
			}
		} else {
			byKind[f.Kind] = f
		}
	}
	out := make([]Factor, 0, len(byKind))
	for _, f := range byKind {
		out = append(out, f)
	}
	// Stable sort by kind for deterministic output.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Kind < out[j].Kind
	})
	return out
}

// --- Internal indexes ---

type edgeIndex struct {
	// bySourceUID: cluster_id → source_uid → edges (forward: parent→child)
	bySourceUID map[int64]map[string][]TopologyEdgeInput
	// byTargetUID: cluster_id → target_uid → edges (reverse: child→parent)
	byTargetUID map[int64]map[string][]TopologyEdgeInput
}

func buildEdgeIndex(edges []TopologyEdgeInput) *edgeIndex {
	idx := &edgeIndex{
		bySourceUID: make(map[int64]map[string][]TopologyEdgeInput),
		byTargetUID: make(map[int64]map[string][]TopologyEdgeInput),
	}
	for _, e := range edges {
		if e.Source.UID == "" {
			continue
		}
		if idx.bySourceUID[e.ClusterID] == nil {
			idx.bySourceUID[e.ClusterID] = make(map[string][]TopologyEdgeInput)
		}
		idx.bySourceUID[e.ClusterID][e.Source.UID] = append(idx.bySourceUID[e.ClusterID][e.Source.UID], e)
		if e.Target.UID != "" {
			if idx.byTargetUID[e.ClusterID] == nil {
				idx.byTargetUID[e.ClusterID] = make(map[string][]TopologyEdgeInput)
			}
			idx.byTargetUID[e.ClusterID][e.Target.UID] = append(idx.byTargetUID[e.ClusterID][e.Target.UID], e)
		}
	}
	return idx
}

// findPath finds a topology path from source to target using bidirectional BFS
// over active edges. Edges are traversed in both directions (forward as
// parent→child and reverse as child→parent) so a Pod can reach its owning
// Deployment via reverse Owns traversal. Returns the edge kinds in path order
// and the edge IDs. Returns empty when no path exists or when either UID is
// empty.
func (idx *edgeIndex) findPath(clusterID int64, source, target ResourceCitation, _ string) ([]string, []int64) {
	if source.UID == "" || target.UID == "" {
		return nil, nil
	}
	if source.UID == target.UID {
		return []string{}, nil // same resource, zero-distance path
	}

	forward := idx.bySourceUID[clusterID]
	reverse := idx.byTargetUID[clusterID]
	if forward == nil && reverse == nil {
		return nil, nil
	}

	type queueItem struct {
		uid     string
		path    []string
		edgeIDs []int64
		visited map[string]bool
	}

	queue := []queueItem{{
		uid:     source.UID,
		path:    []string{},
		edgeIDs: []int64{},
		visited: map[string]bool{source.UID: true},
	}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.uid == target.UID && len(item.path) > 0 {
			return item.path, item.edgeIDs
		}

		// Limit path length to prevent unbounded traversal.
		if len(item.path) >= MaxTopologyPathLen {
			continue
		}

		// Forward traversal: follow edges where current UID is the source.
		if forward != nil {
			for _, edge := range forward[item.uid] {
				if item.visited[edge.Target.UID] {
					continue
				}
				newVisited := make(map[string]bool, len(item.visited)+1)
				for k, v := range item.visited {
					newVisited[k] = v
				}
				newVisited[edge.Target.UID] = true
				queue = append(queue, queueItem{
					uid:     edge.Target.UID,
					path:    append(append([]string{}, item.path...), edge.Kind),
					edgeIDs: append(append([]int64{}, item.edgeIDs...), edge.ID),
					visited: newVisited,
				})
			}
		}
		// Reverse traversal: follow edges where current UID is the target.
		if reverse != nil {
			for _, edge := range reverse[item.uid] {
				if item.visited[edge.Source.UID] {
					continue
				}
				newVisited := make(map[string]bool, len(item.visited)+1)
				for k, v := range item.visited {
					newVisited[k] = v
				}
				newVisited[edge.Source.UID] = true
				queue = append(queue, queueItem{
					uid:     edge.Source.UID,
					path:    append(append([]string{}, item.path...), edge.Kind),
					edgeIDs: append(append([]int64{}, item.edgeIDs...), edge.ID),
					visited: newVisited,
				})
			}
		}
	}

	return nil, nil
}

type changeIndex struct {
	// byCluster: cluster_id → change events (sorted by started_at desc)
	byCluster map[int64][]ChangeEventInput
}

func buildChangeIndex(changes []ChangeEventInput) *changeIndex {
	idx := &changeIndex{byCluster: make(map[int64][]ChangeEventInput)}
	for _, c := range changes {
		idx.byCluster[c.ClusterID] = append(idx.byCluster[c.ClusterID], c)
	}
	// Sort each cluster's changes by started_at descending for stable lookup.
	for k := range idx.byCluster {
		sort.SliceStable(idx.byCluster[k], func(i, j int) bool {
			return idx.byCluster[k][i].StartedAt.After(idx.byCluster[k][j].StartedAt)
		})
	}
	return idx
}

func (idx *changeIndex) lookup(clusterID int64, kinds []string, startTime, endTime time.Time) []ChangeEventInput {
	cluster := idx.byCluster[clusterID]
	kindSet := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}
	var out []ChangeEventInput
	for _, c := range cluster {
		if !kindSet[c.Kind] {
			continue
		}
		// Change must have started within [startTime, endTime].
		if c.StartedAt.Before(startTime) || c.StartedAt.After(endTime) {
			continue
		}
		out = append(out, c)
	}
	return out
}

type diagIndex struct {
	// byClusterUID: cluster_id → resource_uid → diagnoses
	byClusterUID map[int64]map[string][]DiagnosisRef
}

func buildDiagnosisIndex(diagnoses []DiagnosisRef) *diagIndex {
	idx := &diagIndex{byClusterUID: make(map[int64]map[string][]DiagnosisRef)}
	for _, d := range diagnoses {
		if d.Resource.UID == "" {
			continue
		}
		if idx.byClusterUID[d.ClusterID] == nil {
			idx.byClusterUID[d.ClusterID] = make(map[string][]DiagnosisRef)
		}
		idx.byClusterUID[d.ClusterID][d.Resource.UID] = append(idx.byClusterUID[d.ClusterID][d.Resource.UID], d)
	}
	return idx
}

func (idx *diagIndex) lookup(clusterID int64, resource ResourceCitation) []DiagnosisRef {
	if resource.UID == "" {
		return nil
	}
	if cluster := idx.byClusterUID[clusterID]; cluster != nil {
		return cluster[resource.UID]
	}
	return nil
}
