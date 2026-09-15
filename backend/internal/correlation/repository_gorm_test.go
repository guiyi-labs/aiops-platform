package correlation

// Covers the correlation repository: the pure row/JSON conversion helpers and
// the GORM methods driven through sqlmock. The repository emits
// Postgres-specific SQL (RETURNING, ON CONFLICT, JSONB), so an in-memory
// sqlite database is not a valid substitute.

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// --- sqlmock plumbing ---

// normalizeSQL lowercases and collapses whitespace so expectations stay
// readable regardless of how GORM formats the generated statement.
func normalizeSQL(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// looseSQLMatcher matches when the executed statement contains the expected
// fragment. Assertions stay on semantics (table, predicates, ordering) instead
// of GORM's exact formatting.
func looseSQLMatcher(expected, actual string) error {
	if !strings.Contains(normalizeSQL(actual), normalizeSQL(expected)) {
		return fmt.Errorf("expected SQL containing %q, got %q", expected, actual)
	}
	return nil
}

func newMockGormRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(looseSQLMatcher)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	gdb, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open with sqlmock: %v", err)
	}
	return NewGormRepository(gdb), mock
}

// newDryRunGorm builds a handle that builds SQL without executing it, used to
// assert on the statement applyCaseFilter produces.
func newDryRunGorm(t *testing.T) *gorm.DB {
	t.Helper()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	gdb, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: db}), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("gorm.Open dry-run: %v", err)
	}
	return gdb
}

// captureArg accepts any driver value and records it, so a test can assert on a
// payload (e.g. merged factors) without hard-coding GORM's argument order.
type captureArg struct{ seen *[]driver.Value }

func (c captureArg) Match(v driver.Value) bool {
	*c.seen = append(*c.seen, v)
	return true
}

func repeatedArgs(a sqlmock.Argument, n int) []driver.Value {
	out := make([]driver.Value, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, a)
	}
	return out
}

// expectInsertCase / expectInsertRow describe one GORM Create: the statement is
// wrapped in the implicit transaction GORM opens for every write, and it is a
// Query because Postgres uses RETURNING "id".
func expectInsertRow(mock sqlmock.Sqlmock, table string, id int64) {
	mock.ExpectBegin()
	mock.ExpectQuery(`insert into ` + table).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
	mock.ExpectCommit()
}

// expectFindActiveCase describes the dedup lookup UpsertResult issues first.
func expectFindActiveCase(mock sqlmock.Sqlmock, caseKey string, rows *sqlmock.Rows) {
	mock.ExpectQuery(`select * from "correlation_cases" where case_key = $1 and status = $2`).
		WithArgs(caseKey, "active", int64(1)).
		WillReturnRows(rows)
}

var caseRowColumns = []string{
	"id", "case_key", "cluster_id", "rule_id", "correlation_version",
	"primary_kind", "primary_namespace", "primary_name", "primary_uid",
	"primary_incomplete", "status", "confidence", "evidence_completeness",
	"factors", "diagnosis_ids", "root_change_candidate_id",
	"first_observed_at", "last_observed_at", "created_at", "updated_at",
}

var signalLinkRowColumns = []string{
	"id", "case_id", "signal_occurrence_id", "relation", "signal_id",
	"producer", "observed_at", "coverage", "freshness", "window_start",
	"window_end", "created_at",
}

var resourceLinkRowColumns = []string{
	"id", "case_id", "kind", "namespace", "name", "uid", "incomplete",
	"relation", "topology_path", "edge_ids", "created_at",
}

var changeCandidateRowColumns = []string{
	"id", "case_id", "change_event_id", "rule_id", "confidence", "rank",
	"factors", "evidence_refs", "contradicting_refs", "reason_code",
	"created_at", "updated_at",
}

// --- pure helpers: table names, JSONB SQL type, row conversion ---

func TestRepositoryRowTableNames(t *testing.T) {
	got := map[string]string{
		"case":             caseRow{}.TableName(),
		"signal_link":      signalLinkRow{}.TableName(),
		"resource_link":    resourceLinkRow{}.TableName(),
		"change_candidate": changeCandidateRow{}.TableName(),
	}
	want := map[string]string{
		"case":             "correlation_cases",
		"signal_link":      "correlation_signal_links",
		"resource_link":    "correlation_resource_links",
		"change_candidate": "correlation_change_candidates",
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s TableName() = %q, want %q", name, got[name], w)
		}
	}
}

func TestJSONBValueDefaultsToEmptyArray(t *testing.T) {
	var empty JSONB
	v, err := empty.Value()
	if err != nil {
		t.Fatalf("Value() on empty JSONB: %v", err)
	}
	if b, ok := v.([]byte); !ok || string(b) != "[]" {
		t.Fatalf("empty JSONB Value() = %#v, want []byte(\"[]\")", v)
	}

	full := JSONB(`[{"kind":"same_uid"}]`)
	v, err = full.Value()
	if err != nil {
		t.Fatalf("Value() on populated JSONB: %v", err)
	}
	if b, ok := v.([]byte); !ok || string(b) != `[{"kind":"same_uid"}]` {
		t.Fatalf("populated JSONB Value() = %#v", v)
	}
}

func TestJSONBScanAcceptsBytesStringAndNil(t *testing.T) {
	var fromNil JSONB
	if err := fromNil.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	if string(fromNil) != "[]" {
		t.Errorf("Scan(nil) = %q, want []", fromNil)
	}

	var fromBytes JSONB
	if err := fromBytes.Scan([]byte(`[1,2]`)); err != nil {
		t.Fatalf("Scan([]byte): %v", err)
	}
	if string(fromBytes) != "[1,2]" {
		t.Errorf("Scan([]byte) = %q, want [1,2]", fromBytes)
	}

	var fromString JSONB
	if err := fromString.Scan(`["a"]`); err != nil {
		t.Fatalf("Scan(string): %v", err)
	}
	if string(fromString) != `["a"]` {
		t.Errorf("Scan(string) = %q, want [\"a\"]", fromString)
	}

	// Unsupported driver types are ignored rather than corrupting the value.
	var fromInt JSONB
	if err := fromInt.Scan(int64(5)); err != nil {
		t.Fatalf("Scan(int64): %v", err)
	}
	if len(fromInt) != 0 {
		t.Errorf("Scan(int64) = %q, want untouched empty value", fromInt)
	}

	// Scanning copies: mutating the source slice must not alias the value.
	src := []byte(`[3]`)
	var copied JSONB
	if err := copied.Scan(src); err != nil {
		t.Fatalf("Scan(src): %v", err)
	}
	src[1] = '9'
	if string(copied) != "[3]" {
		t.Errorf("Scan aliased the source slice: %q", copied)
	}
}

func TestJSONBMarshalAndUnmarshal(t *testing.T) {
	var empty JSONB
	b, err := empty.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON on empty: %v", err)
	}
	if string(b) != "[]" {
		t.Errorf("empty MarshalJSON = %q, want []", b)
	}

	full := JSONB(`{"a":1}`)
	b, err = full.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(b) != `{"a":1}` {
		t.Errorf("MarshalJSON = %q, want {\"a\":1}", b)
	}

	var decoded JSONB
	if err := decoded.UnmarshalJSON([]byte(`[9]`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if string(decoded) != "[9]" {
		t.Errorf("UnmarshalJSON = %q, want [9]", decoded)
	}

	// A nil receiver is a no-op rather than a panic.
	var nilJSONB *JSONB
	if err := nilJSONB.UnmarshalJSON([]byte(`[1]`)); err != nil {
		t.Fatalf("nil UnmarshalJSON: %v", err)
	}
	if nilJSONB != nil {
		t.Error("nil UnmarshalJSON must not allocate")
	}
}

func TestCaseRowRoundTrip(t *testing.T) {
	rootID := int64(77)
	first := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	last := first.Add(30 * time.Minute)
	in := Case{
		ID:                 5,
		CaseKey:            "case-key-5",
		ClusterID:          3,
		RuleID:             "correlation.rollout_causes_pod_failure.v1",
		CorrelationVersion: CorrelationVersion,
		PrimaryResource: ResourceCitation{
			Kind: "Pod", Namespace: "app", Name: "web-abc", UID: "pod-uid", Incomplete: true,
		},
		Status:                CaseStatusActive,
		Confidence:            ConfidenceCandidate,
		EvidenceCompleteness:  CompletenessPartial,
		Factors:               []Factor{{Kind: "same_uid", Value: "true", Weight: 1}},
		DiagnosisIDs:          []int64{11, 12},
		FirstObservedAt:       first,
		LastObservedAt:        last,
		CreatedAt:             first,
		UpdatedAt:             last,
		RootChangeCandidateID: &rootID,
	}

	row := caseToRow(&in)
	if row.PrimaryKind != "Pod" || row.PrimaryNamespace != "app" || row.PrimaryName != "web-abc" {
		t.Fatalf("primary resource not flattened: %+v", row)
	}
	if !row.PrimaryIncomplete {
		t.Error("PrimaryIncomplete not propagated")
	}
	if row.Status != "active" || row.Confidence != "candidate" || row.EvidenceCompleteness != "partial" {
		t.Fatalf("string columns = %q/%q/%q", row.Status, row.Confidence, row.EvidenceCompleteness)
	}
	if string(row.Factors) != `[{"kind":"same_uid","value":"true","weight":1}]` {
		t.Errorf("Factors = %s", row.Factors)
	}
	if string(row.DiagnosisIDs) != "[11,12]" {
		t.Errorf("DiagnosisIDs = %s", row.DiagnosisIDs)
	}
	if row.RootChangeCandidateID == nil || *row.RootChangeCandidateID != rootID {
		t.Errorf("RootChangeCandidateID = %v, want %d", row.RootChangeCandidateID, rootID)
	}

	back := rowToCase(&row)
	if back.ID != in.ID || back.CaseKey != in.CaseKey || back.ClusterID != in.ClusterID {
		t.Fatalf("identity not preserved: %+v", back)
	}
	if back.PrimaryResource != in.PrimaryResource {
		t.Errorf("primary resource = %+v, want %+v", back.PrimaryResource, in.PrimaryResource)
	}
	if back.Status != in.Status || back.Confidence != in.Confidence ||
		back.EvidenceCompleteness != in.EvidenceCompleteness {
		t.Errorf("enums not preserved: %+v", back)
	}
	if len(back.Factors) != 1 || back.Factors[0].Kind != "same_uid" {
		t.Errorf("factors = %+v", back.Factors)
	}
	if len(back.DiagnosisIDs) != 2 || back.DiagnosisIDs[1] != 12 {
		t.Errorf("diagnosis ids = %v", back.DiagnosisIDs)
	}
	if !back.FirstObservedAt.Equal(first) || !back.LastObservedAt.Equal(last) {
		t.Errorf("window = %v..%v", back.FirstObservedAt, back.LastObservedAt)
	}
}

func TestSignalLinkRowRoundTrip(t *testing.T) {
	observed := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	freshness := observed.Add(-2 * time.Minute)
	windowStart := observed.Add(-10 * time.Minute)
	windowEnd := observed.Add(-time.Minute)
	in := SignalLink{
		ID: 4, CaseID: 9, SignalOccurrenceID: 100,
		Relation: SignalRelationTrigger, SignalID: "diag.pod.crash_loop.v1",
		Producer: "diagnosis", ObservedAt: observed, Coverage: "complete",
		Freshness: &freshness, WindowStart: &windowStart, WindowEnd: &windowEnd,
		CreatedAt: observed,
	}

	row := signalLinkToRow(&in)
	if row.Relation != "trigger" || row.SignalID != "diag.pod.crash_loop.v1" || row.CaseID != 9 {
		t.Fatalf("row = %+v", row)
	}
	if row.Freshness == nil || !row.Freshness.Equal(freshness) {
		t.Errorf("freshness = %v", row.Freshness)
	}

	back := rowToSignalLink(&row)
	if back.ID != in.ID || back.CaseID != in.CaseID || back.SignalOccurrenceID != in.SignalOccurrenceID {
		t.Fatalf("identity not preserved: %+v", back)
	}
	if back.Relation != in.Relation || back.Producer != in.Producer || back.Coverage != in.Coverage {
		t.Errorf("fields not preserved: %+v", back)
	}
	if !back.ObservedAt.Equal(observed) {
		t.Errorf("observed_at = %v", back.ObservedAt)
	}
	if back.WindowStart == nil || !back.WindowStart.Equal(windowStart) ||
		back.WindowEnd == nil || !back.WindowEnd.Equal(windowEnd) {
		t.Errorf("window = %v..%v", back.WindowStart, back.WindowEnd)
	}
}

func TestResourceLinkRowRoundTrip(t *testing.T) {
	created := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	in := ResourceLink{
		ID: 6, CaseID: 9,
		Resource:     ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: "deploy-uid"},
		Relation:     ResourceRelationUpstream,
		TopologyPath: []string{"Owns"},
		EdgeIDs:      []int64{301, 302},
		CreatedAt:    created,
	}

	row := resourceLinkToRow(&in)
	if row.Kind != "Deployment" || row.Namespace != "app" || row.Name != "web" || row.UID != "deploy-uid" {
		t.Fatalf("resource not flattened: %+v", row)
	}
	if row.Relation != "upstream" {
		t.Errorf("relation = %q", row.Relation)
	}
	if string(row.TopologyPath) != `["Owns"]` || string(row.EdgeIDs) != "[301,302]" {
		t.Errorf("path/edges = %s / %s", row.TopologyPath, row.EdgeIDs)
	}

	back := rowToResourceLink(&row)
	if back.ID != in.ID || back.CaseID != in.CaseID || back.Relation != in.Relation {
		t.Fatalf("identity not preserved: %+v", back)
	}
	if back.Resource != in.Resource {
		t.Errorf("resource = %+v, want %+v", back.Resource, in.Resource)
	}
	if len(back.TopologyPath) != 1 || back.TopologyPath[0] != "Owns" {
		t.Errorf("topology path = %v", back.TopologyPath)
	}
	if len(back.EdgeIDs) != 2 || back.EdgeIDs[1] != 302 {
		t.Errorf("edge ids = %v", back.EdgeIDs)
	}
}

func TestChangeCandidateRowRoundTrip(t *testing.T) {
	created := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	in := ChangeCandidate{
		ID: 8, CaseID: 9, ChangeEventID: 200,
		RuleID:     "correlation.rollout_causes_unavailable_deployment.v1",
		Confidence: ConfidenceConfirmed, Rank: 1,
		Factors:           []Factor{{Kind: "change_symptom_rule", Value: "match", Weight: 1}},
		EvidenceRefs:      []EvidenceRef{{Kind: "signal", ID: 100, ContentHash: "abc"}},
		ContradictingRefs: []EvidenceRef{{Kind: "signal", ID: 101}},
		ReasonCode:        "rollout_precedes_pod_failure",
		CreatedAt:         created, UpdatedAt: created,
	}

	row := changeCandidateToRow(&in)
	if row.RuleID != in.RuleID || row.Confidence != "confirmed" || row.Rank != 1 {
		t.Fatalf("row = %+v", row)
	}
	if row.ReasonCode != "rollout_precedes_pod_failure" {
		t.Errorf("reason code = %q", row.ReasonCode)
	}

	back := rowToChangeCandidate(&row)
	if back.ID != in.ID || back.CaseID != in.CaseID || back.ChangeEventID != in.ChangeEventID {
		t.Fatalf("identity not preserved: %+v", back)
	}
	if back.Confidence != ConfidenceConfirmed || back.Rank != 1 {
		t.Errorf("confidence/rank = %s/%d", back.Confidence, back.Rank)
	}
	if len(back.Factors) != 1 || back.Factors[0].Kind != "change_symptom_rule" {
		t.Errorf("factors = %+v", back.Factors)
	}
	if len(back.EvidenceRefs) != 1 || back.EvidenceRefs[0].ID != 100 || back.EvidenceRefs[0].ContentHash != "abc" {
		t.Errorf("evidence refs = %+v", back.EvidenceRefs)
	}
	if len(back.ContradictingRefs) != 1 || back.ContradictingRefs[0].ID != 101 {
		t.Errorf("contradicting refs = %+v", back.ContradictingRefs)
	}
}

func TestUnmarshalHelpersEmptyAndMalformedInput(t *testing.T) {
	if got := unmarshalEvidenceRefs(nil); got != nil {
		t.Errorf("unmarshalEvidenceRefs(nil) = %+v, want nil", got)
	}
	if got := unmarshalEvidenceRefs(JSONB("not-json")); got != nil {
		t.Errorf("unmarshalEvidenceRefs(malformed) = %+v, want nil", got)
	}
	if got := unmarshalInt64s(nil); got != nil {
		t.Errorf("unmarshalInt64s(nil) = %+v, want nil", got)
	}
	if got := unmarshalInt64s(JSONB(`{"nope":1}`)); got != nil {
		t.Errorf("unmarshalInt64s(wrong shape) = %+v, want nil", got)
	}
	if got := unmarshalStrings(nil); got != nil {
		t.Errorf("unmarshalStrings(nil) = %+v, want nil", got)
	}
	if got := unmarshalStrings(JSONB(`not-json`)); got != nil {
		t.Errorf("unmarshalStrings(malformed) = %+v, want nil", got)
	}
}

func TestNopRepositoryContract(t *testing.T) {
	var repo Repository = NopRepository{}
	ctx := context.Background()

	got, err := repo.UpsertResult(ctx, &CorrelationResult{Case: Case{CaseKey: "ignored"}})
	if err != nil || got.ID != 0 {
		t.Errorf("UpsertResult = (%+v, %v), want zero case and nil error", got, err)
	}
	if _, err := repo.GetCase(ctx, 1); !errors.Is(err, ErrCaseNotFound) {
		t.Errorf("GetCase err = %v, want ErrCaseNotFound", err)
	}
	items, total, err := repo.ListCases(ctx, CaseFilter{})
	if err != nil || items != nil || total != 0 {
		t.Errorf("ListCases = (%v, %d, %v)", items, total, err)
	}
	items, total, err = repo.ListTimeline(ctx, CaseFilter{})
	if err != nil || items != nil || total != 0 {
		t.Errorf("ListTimeline = (%v, %d, %v)", items, total, err)
	}
	if links, err := repo.ListSignalLinks(ctx, 1); err != nil || links != nil {
		t.Errorf("ListSignalLinks = (%v, %v)", links, err)
	}
	if links, err := repo.ListResourceLinks(ctx, 1); err != nil || links != nil {
		t.Errorf("ListResourceLinks = (%v, %v)", links, err)
	}
	if cands, err := repo.ListChangeCandidates(ctx, 1); err != nil || cands != nil {
		t.Errorf("ListChangeCandidates = (%v, %v)", cands, err)
	}
	if err := repo.ResolveCaseStatus(ctx, 1, CaseStatusResolved, time.Now()); err != nil {
		t.Errorf("ResolveCaseStatus = %v, want nil", err)
	}
}

// --- applyCaseFilter ---

func TestApplyCaseFilterBuildsEveryPredicate(t *testing.T) {
	gdb := newDryRunGorm(t)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	filter := CaseFilter{
		ClusterID: 7, Namespace: "app", RuleID: "r1",
		Status: CaseStatusActive, Confidence: ConfidenceConfirmed,
		PrimaryKind: "Pod", PrimaryUID: "uid-1",
		StartTime: &start, EndTime: &end, Limit: 10,
	}

	q := applyCaseFilter(gdb.Model(&caseRow{}), filter)
	var rows []caseRow
	if err := q.Find(&rows).Error; err != nil {
		t.Fatalf("dry-run Find: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("dry run returned %d rows", len(rows))
	}
	built := normalizeSQL(q.Statement.SQL.String())
	for _, want := range []string{
		`from "correlation_cases"`,
		"cluster_id = $", "primary_namespace = $", "rule_id = $", "status = $",
		"confidence = $", "primary_kind = $", "primary_uid = $",
		"last_observed_at >= $", "first_observed_at <= $",
	} {
		if !strings.Contains(built, want) {
			t.Errorf("filtered SQL missing %q:\n%s", want, built)
		}
	}
}

func TestApplyCaseFilterEmptyAddsNoPredicates(t *testing.T) {
	gdb := newDryRunGorm(t)
	q := applyCaseFilter(gdb.Model(&caseRow{}), CaseFilter{})
	var rows []caseRow
	if err := q.Find(&rows).Error; err != nil {
		t.Fatalf("dry-run Find: %v", err)
	}
	built := normalizeSQL(q.Statement.SQL.String())
	if !strings.Contains(built, `from "correlation_cases"`) {
		t.Fatalf("unexpected SQL: %s", built)
	}
	if strings.Contains(built, "where") {
		t.Errorf("empty filter must not add predicates: %s", built)
	}
}

// --- GORM repository ---

func TestNewGormRepositoryKeepsHandle(t *testing.T) {
	gdb := newDryRunGorm(t)
	repo := NewGormRepository(gdb)
	if repo.db != gdb {
		t.Fatal("NewGormRepository did not retain the gorm handle")
	}
	var _ Repository = repo
}

func TestGormRepositoryUpsertResultInsertsNewCaseAndLinks(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

	expectFindActiveCase(mock, "case-key-1", sqlmock.NewRows(caseRowColumns))
	expectInsertRow(mock, `"correlation_cases"`, 42)
	expectInsertRow(mock, `"correlation_signal_links"`, 1)
	expectInsertRow(mock, `"correlation_resource_links"`, 2)
	expectInsertRow(mock, `"correlation_change_candidates"`, 3)
	mock.ExpectQuery(`select * from "correlation_change_candidates" where case_id = $1 and confidence = $2`).
		WithArgs(int64(42), "confirmed", int64(1)).
		WillReturnRows(sqlmock.NewRows(changeCandidateRowColumns).AddRow(
			3, 42, 200, "rule", "confirmed", 1, `[]`, `[]`, `[]`, "reason", now, now))
	mock.ExpectBegin()
	mock.ExpectExec(`update "correlation_cases" set "root_change_candidate_id"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rootID := int64(3)
	result := &CorrelationResult{
		Case: Case{
			CaseKey: "case-key-1", ClusterID: 3, RuleID: "rule",
			CorrelationVersion: CorrelationVersion,
			PrimaryResource:    ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web", UID: "uid"},
			Status:             CaseStatusActive, Confidence: ConfidenceConfirmed,
			EvidenceCompleteness:  CompletenessComplete,
			Factors:               []Factor{{Kind: "same_uid", Weight: 1}},
			FirstObservedAt:       now,
			LastObservedAt:        now,
			RootChangeCandidateID: &rootID,
		},
		SignalLinks:      []SignalLink{{SignalOccurrenceID: 100, Relation: SignalRelationTrigger, SignalID: "sig", ObservedAt: now}},
		ResourceLinks:    []ResourceLink{{Resource: ResourceCitation{Kind: "Deployment", Name: "web"}, Relation: ResourceRelationUpstream}},
		ChangeCandidates: []ChangeCandidate{{ChangeEventID: 200, RuleID: "rule", Confidence: ConfidenceConfirmed, Rank: 1}},
	}

	got, err := repo.UpsertResult(ctx, result)
	if err != nil {
		t.Fatalf("UpsertResult: %v", err)
	}
	if got.ID != 42 {
		t.Fatalf("persisted case id = %d, want 42", got.ID)
	}
	// The persisted ID must be wired back into every link/candidate so the
	// service can hand them to the API without a second read.
	if result.SignalLinks[0].CaseID != 42 {
		t.Errorf("signal link case_id = %d, want 42", result.SignalLinks[0].CaseID)
	}
	if result.ResourceLinks[0].CaseID != 42 {
		t.Errorf("resource link case_id = %d, want 42", result.ResourceLinks[0].CaseID)
	}
	if result.ChangeCandidates[0].CaseID != 42 {
		t.Errorf("change candidate case_id = %d, want 42", result.ChangeCandidates[0].CaseID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryUpsertResultWithoutRootCandidateSkipsLookup(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

	expectFindActiveCase(mock, "case-key-2", sqlmock.NewRows(caseRowColumns))
	expectInsertRow(mock, `"correlation_cases"`, 7)

	got, err := repo.UpsertResult(context.Background(), &CorrelationResult{
		Case: Case{
			CaseKey: "case-key-2", Status: CaseStatusActive, Confidence: ConfidenceCandidate,
			EvidenceCompleteness: CompletenessPartial,
			FirstObservedAt:      now, LastObservedAt: now,
		},
	})
	if err != nil {
		t.Fatalf("UpsertResult: %v", err)
	}
	if got.ID != 7 {
		t.Fatalf("persisted case id = %d, want 7", got.ID)
	}
	// No RootChangeCandidateID and no links: nothing else may hit the database.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryUpsertResultMergesExistingActiveCase(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	existingFirst := now.Add(-time.Hour)
	existingLast := now.Add(-10 * time.Minute)

	expectFindActiveCase(mock, "case-key-3", sqlmock.NewRows(caseRowColumns).AddRow(
		9, "case-key-3", 3, "rule", CorrelationVersion, "Pod", "app", "web", "uid",
		false, "active", "unknown", "insufficient",
		`[{"kind":"same_uid","value":"true","weight":1}]`, `[11]`, nil,
		existingFirst, existingLast, existingFirst, existingLast))

	var seen []driver.Value
	capture := captureArg{seen: &seen}
	// Save issues a full-row UPDATE (19 columns + the primary-key predicate).
	mock.ExpectBegin()
	mock.ExpectExec(`update "correlation_cases" set "case_key"`).
		WithArgs(repeatedArgs(capture, 20)...).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	got, err := repo.UpsertResult(context.Background(), &CorrelationResult{
		Case: Case{
			CaseKey: "case-key-3", ClusterID: 3, RuleID: "rule",
			CorrelationVersion: CorrelationVersion,
			PrimaryResource:    ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web", UID: "uid"},
			Status:             CaseStatusActive, Confidence: ConfidenceConfirmed,
			EvidenceCompleteness: CompletenessComplete,
			Factors:              []Factor{{Kind: "topology_distance", Weight: 0.5}},
			DiagnosisIDs:         []int64{12},
			FirstObservedAt:      now.Add(-2 * time.Hour),
			LastObservedAt:       now,
		},
	})
	if err != nil {
		t.Fatalf("UpsertResult: %v", err)
	}
	if got.ID != 9 {
		t.Fatalf("merged case id = %d, want the existing 9", got.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	// The merged payload must retain the pre-existing factor and add the new
	// one, and the widened window must be persisted.
	var factors, firstAt, lastAt, confidence, completeness string
	for _, v := range seen {
		switch tv := v.(type) {
		case []byte:
			s := string(tv)
			if strings.HasPrefix(s, "[{") {
				factors += s
			}
		case string:
			switch tv {
			case "confirmed":
				confidence = tv
			case "complete":
				completeness = tv
			}
		case time.Time:
			switch {
			case tv.Equal(now.Add(-2 * time.Hour)):
				firstAt = tv.Format(time.RFC3339)
			case tv.Equal(now):
				lastAt = tv.Format(time.RFC3339)
			}
		}
	}
	if !strings.Contains(factors, "same_uid") || !strings.Contains(factors, "topology_distance") {
		t.Errorf("merged factors not persisted: %q (all args: %+v)", factors, seen)
	}
	if confidence != "confirmed" {
		t.Errorf("confidence was not promoted: args %+v", seen)
	}
	if completeness != "complete" {
		t.Errorf("completeness was not promoted: args %+v", seen)
	}
	if firstAt == "" {
		t.Errorf("observed window was not widened backwards: args %+v", seen)
	}
	if lastAt == "" {
		t.Errorf("observed window was not widened forwards: args %+v", seen)
	}
}

func TestGormRepositoryUpsertResultPropagatesLookupError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("connection reset")
	mock.ExpectQuery(`select * from "correlation_cases" where case_key = $1 and status = $2`).
		WillReturnError(boom)

	got, err := repo.UpsertResult(context.Background(), &CorrelationResult{
		Case: Case{CaseKey: "k", Status: CaseStatusActive},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the lookup error", err)
	}
	if got.ID != 0 {
		t.Errorf("case = %+v, want zero value", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryUpsertResultPropagatesInsertError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("unique violation")
	expectFindActiveCase(mock, "k", sqlmock.NewRows(caseRowColumns))
	mock.ExpectBegin()
	mock.ExpectQuery(`insert into "correlation_cases"`).WillReturnError(boom)
	mock.ExpectRollback()

	if _, err := repo.UpsertResult(context.Background(), &CorrelationResult{
		Case: Case{CaseKey: "k", Status: CaseStatusActive},
	}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the insert error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryUpsertResultPropagatesLinkInsertError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("link insert failed")
	expectFindActiveCase(mock, "k", sqlmock.NewRows(caseRowColumns))
	expectInsertRow(mock, `"correlation_cases"`, 5)
	mock.ExpectBegin()
	mock.ExpectQuery(`insert into "correlation_signal_links"`).WillReturnError(boom)
	mock.ExpectRollback()

	_, err := repo.UpsertResult(context.Background(), &CorrelationResult{
		Case:        Case{CaseKey: "k", Status: CaseStatusActive},
		SignalLinks: []SignalLink{{SignalOccurrenceID: 1, Relation: SignalRelationTrigger}},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the link insert error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryUpsertResultIgnoresMissingRootCandidate(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	expectFindActiveCase(mock, "k", sqlmock.NewRows(caseRowColumns))
	expectInsertRow(mock, `"correlation_cases"`, 11)
	// No confirmed candidate exists yet: the lookup returns no rows and the
	// case is still persisted without a root candidate pointer.
	mock.ExpectQuery(`select * from "correlation_change_candidates" where case_id = $1 and confidence = $2`).
		WithArgs(int64(11), "confirmed", int64(1)).
		WillReturnRows(sqlmock.NewRows(changeCandidateRowColumns))

	rootID := int64(99)
	got, err := repo.UpsertResult(context.Background(), &CorrelationResult{
		Case: Case{CaseKey: "k", Status: CaseStatusActive, RootChangeCandidateID: &rootID},
	})
	if err != nil {
		t.Fatalf("UpsertResult: %v", err)
	}
	if got.ID != 11 {
		t.Fatalf("case id = %d, want 11", got.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetCaseAssemblesView(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`select * from "correlation_cases" where id = $1`).
		WithArgs(int64(5), int64(1)).
		WillReturnRows(sqlmock.NewRows(caseRowColumns).AddRow(
			5, "case-key-5", 3, "rule", CorrelationVersion, "Pod", "app", "web", "uid",
			false, "active", "candidate", "partial",
			`[{"kind":"same_uid","value":"true","weight":1}]`, `[11]`, nil,
			now, now, now, now))
	mock.ExpectQuery(`select * from "correlation_signal_links" where case_id = $1 order by observed_at desc, id desc`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows(signalLinkRowColumns).AddRow(
			1, 5, 100, "trigger", "sig", "diagnosis", now, "complete", nil, nil, nil, now))
	mock.ExpectQuery(`select * from "correlation_resource_links" where case_id = $1 order by relation asc, id asc`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows(resourceLinkRowColumns).AddRow(
			2, 5, "Deployment", "app", "web", "deploy-uid", false, "upstream", `["Owns"]`, `[301]`, now))
	mock.ExpectQuery(`select * from "correlation_change_candidates" where case_id = $1 order by rank asc, id asc`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows(changeCandidateRowColumns).AddRow(
			3, 5, 200, "rule", "candidate", 1, `[]`, `[]`, `[]`, "reason", now, now))

	view, err := repo.GetCase(context.Background(), 5)
	if err != nil {
		t.Fatalf("GetCase: %v", err)
	}
	if view.Case.ID != 5 || view.Case.Status != CaseStatusActive || view.Case.Confidence != ConfidenceCandidate {
		t.Fatalf("case = %+v", view.Case)
	}
	if len(view.Case.Factors) != 1 || view.Case.Factors[0].Kind != "same_uid" {
		t.Errorf("factors = %+v", view.Case.Factors)
	}
	if len(view.SignalLinks) != 1 || view.SignalLinks[0].Relation != SignalRelationTrigger {
		t.Errorf("signal links = %+v", view.SignalLinks)
	}
	if len(view.ResourceLinks) != 1 || view.ResourceLinks[0].Relation != ResourceRelationUpstream {
		t.Errorf("resource links = %+v", view.ResourceLinks)
	}
	if len(view.ChangeCandidates) != 1 || view.ChangeCandidates[0].Rank != 1 {
		t.Errorf("change candidates = %+v", view.ChangeCandidates)
	}
	if view.GeneratedAt.IsZero() {
		t.Error("GeneratedAt must be set")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetCaseNotFound(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	mock.ExpectQuery(`select * from "correlation_cases" where id = $1`).
		WithArgs(int64(404), int64(1)).
		WillReturnRows(sqlmock.NewRows(caseRowColumns))

	if _, err := repo.GetCase(context.Background(), 404); !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("err = %v, want ErrCaseNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetCasePropagatesQueryError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("db unavailable")
	mock.ExpectQuery(`select * from "correlation_cases" where id = $1`).WillReturnError(boom)

	_, err := repo.GetCase(context.Background(), 1)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the driver error", err)
	}
	if errors.Is(err, ErrCaseNotFound) {
		t.Error("a driver error must not be reported as ErrCaseNotFound")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetCasePropagatesLinkError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("link query failed")
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`select * from "correlation_cases" where id = $1`).
		WillReturnRows(sqlmock.NewRows(caseRowColumns).AddRow(
			5, "k", 3, "rule", CorrelationVersion, "Pod", "app", "web", "uid",
			false, "active", "candidate", "partial", `[]`, `[]`, nil, now, now, now, now))
	mock.ExpectQuery(`select * from "correlation_signal_links" where case_id = $1`).WillReturnError(boom)

	if _, err := repo.GetCase(context.Background(), 5); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the link query error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListCases(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`select count(*) from "correlation_cases" where cluster_id = $1`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`select * from "correlation_cases" where cluster_id = $1 order by last_observed_at desc, id desc`).
		WithArgs(int64(3), 100).
		WillReturnRows(sqlmock.NewRows(caseRowColumns).
			AddRow(2, "k2", 3, "rule", CorrelationVersion, "Pod", "app", "web-2", "uid-2",
				false, "active", "confirmed", "complete", `[]`, `[]`, nil, now, now, now, now).
			AddRow(1, "k1", 3, "rule", CorrelationVersion, "Pod", "app", "web-1", "uid-1",
				false, "active", "candidate", "partial", `[]`, `[]`, nil, now, now, now, now))

	items, total, err := repo.ListCases(context.Background(), CaseFilter{ClusterID: 3})
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("total = %d, items = %d, want 2/2", total, len(items))
	}
	if items[0].ID != 2 || items[1].ID != 1 {
		t.Errorf("ordering not preserved: %+v", items)
	}
	if items[0].Confidence != ConfidenceConfirmed || items[1].Confidence != ConfidenceCandidate {
		t.Errorf("confidence not mapped: %+v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListCasesLimitClamping(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested int
		wantArg   driver.Value
	}{
		{name: "zero uses default", requested: 0, wantArg: 100},
		{name: "negative uses default", requested: -5, wantArg: 100},
		{name: "over max uses default", requested: 500, wantArg: 100},
		{name: "in range preserved", requested: 25, wantArg: 25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := newMockGormRepository(t)
			mock.ExpectQuery(`select count(*) from "correlation_cases"`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery(`select * from "correlation_cases" order by last_observed_at desc, id desc`).
				WithArgs(tc.wantArg).
				WillReturnRows(sqlmock.NewRows(caseRowColumns))

			items, total, err := repo.ListCases(context.Background(), CaseFilter{Limit: tc.requested})
			if err != nil {
				t.Fatalf("ListCases: %v", err)
			}
			if total != 0 || len(items) != 0 {
				t.Fatalf("expected empty result, got %d items / total %d", len(items), total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet expectations: %v", err)
			}
		})
	}
}

func TestGormRepositoryListCasesCountError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("count failed")
	mock.ExpectQuery(`select count(*) from "correlation_cases"`).WillReturnError(boom)

	items, total, err := repo.ListCases(context.Background(), CaseFilter{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the count error", err)
	}
	if items != nil || total != 0 {
		t.Errorf("items = %v, total = %d, want nil/0", items, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListCasesFindError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("select failed")
	mock.ExpectQuery(`select count(*) from "correlation_cases"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`select * from "correlation_cases" order by last_observed_at desc, id desc`).
		WillReturnError(boom)

	items, total, err := repo.ListCases(context.Background(), CaseFilter{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the select error", err)
	}
	if items != nil || total != 0 {
		t.Errorf("items = %v, total = %d, want nil/0", items, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListTimeline(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`select count(*) from "correlation_cases" where primary_uid = $1`).
		WithArgs("uid-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`select * from "correlation_cases" where primary_uid = $1 order by first_observed_at asc, id asc`).
		WithArgs("uid-1", 100).
		WillReturnRows(sqlmock.NewRows(caseRowColumns).AddRow(
			1, "k1", 3, "rule", CorrelationVersion, "Pod", "app", "web-1", "uid-1",
			false, "active", "candidate", "partial", `[]`, `[]`, nil, now, now, now, now))

	items, total, err := repo.ListTimeline(context.Background(), CaseFilter{PrimaryUID: "uid-1"})
	if err != nil {
		t.Fatalf("ListTimeline: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].PrimaryResource.UID != "uid-1" {
		t.Fatalf("timeline = %+v (total %d)", items, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListTimelineErrors(t *testing.T) {
	t.Run("count error", func(t *testing.T) {
		repo, mock := newMockGormRepository(t)
		boom := errors.New("count failed")
		mock.ExpectQuery(`select count(*) from "correlation_cases"`).WillReturnError(boom)
		if _, _, err := repo.ListTimeline(context.Background(), CaseFilter{}); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the count error", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("find error", func(t *testing.T) {
		repo, mock := newMockGormRepository(t)
		boom := errors.New("select failed")
		mock.ExpectQuery(`select count(*) from "correlation_cases"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`select * from "correlation_cases" order by first_observed_at asc, id asc`).
			WillReturnError(boom)
		if _, _, err := repo.ListTimeline(context.Background(), CaseFilter{}); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the select error", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})
}

func TestGormRepositoryListSignalLinks(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`select * from "correlation_signal_links" where case_id = $1 order by observed_at desc, id desc`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows(signalLinkRowColumns).
			AddRow(2, 9, 101, "context", "sig-b", "slo", now, "partial", nil, nil, nil, now).
			AddRow(1, 9, 100, "trigger", "sig-a", "diagnosis", now, "complete", nil, nil, nil, now))

	links, err := repo.ListSignalLinks(context.Background(), 9)
	if err != nil {
		t.Fatalf("ListSignalLinks: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("links = %+v", links)
	}
	if links[0].Relation != SignalRelationContext || links[0].Coverage != "partial" {
		t.Errorf("link[0] = %+v", links[0])
	}
	if links[1].SignalID != "sig-a" || links[1].SignalOccurrenceID != 100 {
		t.Errorf("link[1] = %+v", links[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListSignalLinksError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("query failed")
	mock.ExpectQuery(`select * from "correlation_signal_links" where case_id = $1`).WillReturnError(boom)

	links, err := repo.ListSignalLinks(context.Background(), 9)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the query error", err)
	}
	if links != nil {
		t.Errorf("links = %v, want nil", links)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListResourceLinks(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`select * from "correlation_resource_links" where case_id = $1 order by relation asc, id asc`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows(resourceLinkRowColumns).AddRow(
			1, 9, "Deployment", "app", "web", "deploy-uid", false, "upstream", `["Owns"]`, `[301]`, now))

	links, err := repo.ListResourceLinks(context.Background(), 9)
	if err != nil {
		t.Fatalf("ListResourceLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("links = %+v", links)
	}
	if links[0].Resource.Kind != "Deployment" || links[0].Relation != ResourceRelationUpstream {
		t.Errorf("link = %+v", links[0])
	}
	if len(links[0].TopologyPath) != 1 || links[0].TopologyPath[0] != "Owns" {
		t.Errorf("topology path = %v", links[0].TopologyPath)
	}
	if len(links[0].EdgeIDs) != 1 || links[0].EdgeIDs[0] != 301 {
		t.Errorf("edge ids = %v", links[0].EdgeIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListResourceLinksError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("query failed")
	mock.ExpectQuery(`select * from "correlation_resource_links" where case_id = $1`).WillReturnError(boom)

	if _, err := repo.ListResourceLinks(context.Background(), 9); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the query error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListChangeCandidates(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`select * from "correlation_change_candidates" where case_id = $1 order by rank asc, id asc`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows(changeCandidateRowColumns).
			AddRow(2, 9, 201, "rule-b", "candidate", 2, `[]`, `[]`, `[]`, "reason-b", now, now).
			AddRow(1, 9, 200, "rule-a", "confirmed", 1, `[]`, `[]`, `[]`, "reason-a", now, now))

	cands, err := repo.ListChangeCandidates(context.Background(), 9)
	if err != nil {
		t.Fatalf("ListChangeCandidates: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates = %+v", cands)
	}
	if cands[0].Rank != 2 || cands[1].Rank != 1 {
		t.Errorf("rank ordering not preserved: %+v", cands)
	}
	if cands[1].Confidence != ConfidenceConfirmed || cands[1].ReasonCode != "reason-a" {
		t.Errorf("candidate[1] = %+v", cands[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListChangeCandidatesError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("query failed")
	mock.ExpectQuery(`select * from "correlation_change_candidates" where case_id = $1`).WillReturnError(boom)

	if _, err := repo.ListChangeCandidates(context.Background(), 9); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the query error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryUpsertResultPropagatesMergeSaveError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("update failed")
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	expectFindActiveCase(mock, "k", sqlmock.NewRows(caseRowColumns).AddRow(
		9, "k", 3, "rule", CorrelationVersion, "Pod", "app", "web", "uid",
		false, "active", "unknown", "insufficient", `[]`, `[]`, nil, now, now, now, now))
	mock.ExpectBegin()
	mock.ExpectExec(`update "correlation_cases" set "case_key"`).WillReturnError(boom)
	mock.ExpectRollback()

	_, err := repo.UpsertResult(context.Background(), &CorrelationResult{
		Case: Case{CaseKey: "k", Status: CaseStatusActive, FirstObservedAt: now, LastObservedAt: now},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the merge-save error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryUpsertResultPropagatesResourceAndCandidateInsertErrors(t *testing.T) {
	t.Run("resource link", func(t *testing.T) {
		repo, mock := newMockGormRepository(t)
		boom := errors.New("resource link insert failed")
		expectFindActiveCase(mock, "k", sqlmock.NewRows(caseRowColumns))
		expectInsertRow(mock, `"correlation_cases"`, 5)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into "correlation_resource_links"`).WillReturnError(boom)
		mock.ExpectRollback()

		_, err := repo.UpsertResult(context.Background(), &CorrelationResult{
			Case:          Case{CaseKey: "k", Status: CaseStatusActive},
			ResourceLinks: []ResourceLink{{Resource: ResourceCitation{Kind: "Deployment", Name: "web"}}},
		})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the resource link insert error", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("change candidate", func(t *testing.T) {
		repo, mock := newMockGormRepository(t)
		boom := errors.New("candidate insert failed")
		expectFindActiveCase(mock, "k", sqlmock.NewRows(caseRowColumns))
		expectInsertRow(mock, `"correlation_cases"`, 5)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into "correlation_change_candidates"`).WillReturnError(boom)
		mock.ExpectRollback()

		_, err := repo.UpsertResult(context.Background(), &CorrelationResult{
			Case:             Case{CaseKey: "k", Status: CaseStatusActive},
			ChangeCandidates: []ChangeCandidate{{ChangeEventID: 200, Rank: 1}},
		})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the candidate insert error", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})
}

func TestGormRepositoryGetCasePropagatesResourceAndCandidateErrors(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	caseRows := func() *sqlmock.Rows {
		return sqlmock.NewRows(caseRowColumns).AddRow(
			5, "k", 3, "rule", CorrelationVersion, "Pod", "app", "web", "uid",
			false, "active", "candidate", "partial", `[]`, `[]`, nil, now, now, now, now)
	}

	t.Run("resource links", func(t *testing.T) {
		repo, mock := newMockGormRepository(t)
		boom := errors.New("resource link query failed")
		mock.ExpectQuery(`select * from "correlation_cases" where id = $1`).WillReturnRows(caseRows())
		mock.ExpectQuery(`select * from "correlation_signal_links" where case_id = $1`).
			WillReturnRows(sqlmock.NewRows(signalLinkRowColumns))
		mock.ExpectQuery(`select * from "correlation_resource_links" where case_id = $1`).WillReturnError(boom)

		if _, err := repo.GetCase(context.Background(), 5); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the resource link query error", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("change candidates", func(t *testing.T) {
		repo, mock := newMockGormRepository(t)
		boom := errors.New("candidate query failed")
		mock.ExpectQuery(`select * from "correlation_cases" where id = $1`).WillReturnRows(caseRows())
		mock.ExpectQuery(`select * from "correlation_signal_links" where case_id = $1`).
			WillReturnRows(sqlmock.NewRows(signalLinkRowColumns))
		mock.ExpectQuery(`select * from "correlation_resource_links" where case_id = $1`).
			WillReturnRows(sqlmock.NewRows(resourceLinkRowColumns))
		mock.ExpectQuery(`select * from "correlation_change_candidates" where case_id = $1`).WillReturnError(boom)

		if _, err := repo.GetCase(context.Background(), 5); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the candidate query error", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})
}

func TestMustMarshalJSONBUnsupportedTypeFallsBackToEmptyArray(t *testing.T) {
	// json.Marshal cannot encode a channel, so the helper must fall back to an
	// empty JSON array rather than propagating a corrupt payload.
	if got := mustMarshalJSONB(make(chan int)); string(got) != "[]" {
		t.Fatalf("mustMarshalJSONB(chan) = %q, want []", got)
	}
}

func TestGormRepositoryResolveCaseStatus(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(`update "correlation_cases" set "status"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.ResolveCaseStatus(context.Background(), 5, CaseStatusResolved, now); err != nil {
		t.Fatalf("ResolveCaseStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryResolveCaseStatusMissingCase(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	// Zero rows affected means the case does not exist.
	mock.ExpectBegin()
	mock.ExpectExec(`update "correlation_cases" set "status"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.ResolveCaseStatus(context.Background(), 404, CaseStatusResolved, time.Now())
	if !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("err = %v, want ErrCaseNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryResolveCaseStatusPropagatesError(t *testing.T) {
	repo, mock := newMockGormRepository(t)
	boom := errors.New("update failed")
	mock.ExpectBegin()
	mock.ExpectExec(`update "correlation_cases" set "status"`).WillReturnError(boom)
	mock.ExpectRollback()

	err := repo.ResolveCaseStatus(context.Background(), 5, CaseStatusStale, time.Now())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the update error", err)
	}
	if errors.Is(err, ErrCaseNotFound) {
		t.Error("a driver error must not be reported as ErrCaseNotFound")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
