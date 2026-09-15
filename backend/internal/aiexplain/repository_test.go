package aiexplain

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The aiexplain repository issues Postgres-only SQL (date_trunc, advisory
// locks, CAST(? AS JSONB), FILTER, RETURNING), so the tests below run against
// sqlmock instead of an in-memory database.

var sqlWhitespace = regexp.MustCompile(`\s+`)

func normalizeSQL(query string) string {
	return strings.TrimSpace(sqlWhitespace.ReplaceAllString(query, " "))
}

// sqlFragmentMatcher accepts an expected SQL *fragment*: the statement GORM
// actually issues must contain it once whitespace is collapsed. Assertions
// stay focused on behaviour instead of pinning the entire statement text.
var sqlFragmentMatcher = sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
	expect, actual := normalizeSQL(expectedSQL), normalizeSQL(actualSQL)
	if !strings.Contains(actual, expect) {
		return fmt.Errorf("actual sql %q does not contain expected fragment %q", actual, expect)
	}
	return nil
})

// newMockGorm builds a GORM handle backed by sqlmock, mirroring the existing
// knowledge package test harness.
func newMockGorm(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlFragmentMatcher))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	gdb, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open with sqlmock: %v", err)
	}
	return gdb, mock
}

func mustExpectationsMet(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

var listColumns = []string{
	"id", "diagnosis_id", "actor_user_id", "actor_name", "provider", "model", "provider_response_id",
	"summary", "analysis", "recommended_actions", "citations", "input_tokens", "output_tokens", "created_at",
	"feedback_total", "feedback_helpful", "feedback_partially_helpful", "feedback_not_helpful",
	"my_feedback_id", "my_feedback_verdict", "my_feedback_comment", "my_feedback_actor_name", "my_feedback_created_at",
}

// ---------------------------------------------------------------------------
// pure helpers
// ---------------------------------------------------------------------------

func TestFeedbackSummaryAndHelpfulRate(t *testing.T) {
	summary := feedbackSummary(4, 3, 1, 0)
	if summary.Total != 4 || summary.Helpful != 3 || summary.PartiallyHelpful != 1 || summary.NotHelpful != 0 {
		t.Fatalf("feedbackSummary() = %#v", summary)
	}
	if summary.HelpfulRate != 0.75 {
		t.Fatalf("helpful rate = %v, want 0.75", summary.HelpfulRate)
	}

	empty := feedbackSummary(0, 0, 0, 0)
	if empty.HelpfulRate != 0 {
		t.Fatalf("empty helpful rate = %v, want 0", empty.HelpfulRate)
	}
	if rate := helpfulRate(5, 2); rate != 0.4 {
		t.Fatalf("helpfulRate(5, 2) = %v, want 0.4", rate)
	}
}

func TestNullableIDMapsZeroToNull(t *testing.T) {
	if got := nullableID(0); got != nil {
		t.Fatalf("nullableID(0) = %#v, want nil", got)
	}
	if got := nullableID(12); got != int64(12) {
		t.Fatalf("nullableID(12) = %#v, want int64(12)", got)
	}
}

func TestNewGormRepositoryExposesHandle(t *testing.T) {
	gdb, _ := newMockGorm(t)
	repo := NewGormRepository(gdb)
	if repo == nil || repo.db != gdb {
		t.Fatalf("NewGormRepository() = %#v", repo)
	}
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

func TestGormRepositoryUsageAggregatesAndReportsLastSuccess(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	last := time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)

	mock.ExpectQuery(`COALESCE((SELECT SUM(reserved_tokens) FROM ai_usage_reservations WHERE expires_at > NOW()), 0) AS reserved_tokens`).
		WillReturnRows(sqlmock.NewRows([]string{"used_tokens_today", "reserved_tokens", "explanation_count", "last_success_at"}).
			AddRow(1500, 250, 4, last))

	usage, err := repo.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if usage.UsedTokensToday != 1500 || usage.ReservedTokens != 250 || usage.ExplanationCount != 4 {
		t.Fatalf("usage = %#v", usage)
	}
	if usage.LastSuccessAt == nil || !usage.LastSuccessAt.Equal(last) {
		t.Fatalf("last success at = %v, want %v", usage.LastSuccessAt, last)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryUsageWithoutHistoryLeavesLastSuccessNil(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`AS used_tokens_today`).
		WillReturnRows(sqlmock.NewRows([]string{"used_tokens_today", "reserved_tokens", "explanation_count", "last_success_at"}).
			AddRow(0, 0, 0, nil))

	usage, err := repo.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if usage.LastSuccessAt != nil {
		t.Fatalf("last success at = %v, want nil", usage.LastSuccessAt)
	}
	if usage.UsedTokensToday != 0 || usage.ExplanationCount != 0 {
		t.Fatalf("usage = %#v", usage)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryUsagePropagatesQueryError(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	failure := errors.New("usage query failed")

	mock.ExpectQuery(`AS used_tokens_today`).WillReturnError(failure)

	if _, err := repo.Usage(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Usage() error = %v, want %v", err, failure)
	}
	mustExpectationsMet(t, mock)
}

// ---------------------------------------------------------------------------
// Reserve / Release
// ---------------------------------------------------------------------------

func TestGormRepositoryReserveWritesReservationInsideTransaction(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	expires := time.Date(2026, 8, 12, 10, 5, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock($1)`).
		WithArgs(int64(741009)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM ai_usage_reservations WHERE expires_at <= NOW()`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery(`SELECT COALESCE((SELECT SUM(input_tokens + output_tokens) FROM ai_explanations WHERE created_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'), 0), COALESCE((SELECT SUM(reserved_tokens) FROM ai_usage_reservations), 0)`).
		WillReturnRows(sqlmock.NewRows([]string{"committed", "reserved"}).AddRow(900, 100))
	mock.ExpectExec(`INSERT INTO ai_usage_reservations (id, diagnosis_id, reserved_tokens, expires_at) VALUES ($1, $2, $3, $4)`).
		WithArgs("reservation-1", int64(7), 200, expires).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.Reserve(context.Background(), Reservation{ID: "reservation-1", DiagnosisID: 7, ReservedTokens: 200, ExpiresAt: expires}, 5000)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryReserveRejectsOverBudget(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	expires := time.Date(2026, 8, 12, 10, 5, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock($1)`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM ai_usage_reservations WHERE expires_at <= NOW()`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT COALESCE((SELECT SUM(input_tokens + output_tokens) FROM ai_explanations`).
		WillReturnRows(sqlmock.NewRows([]string{"committed", "reserved"}).AddRow(900, 200))
	mock.ExpectRollback()

	err := repo.Reserve(context.Background(), Reservation{ID: "reservation-2", DiagnosisID: 7, ReservedTokens: 100, ExpiresAt: expires}, 1000)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Reserve() error = %v, want ErrBudgetExceeded", err)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryReserveSkipsBudgetCheckWhenBudgetDisabled(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	expires := time.Date(2026, 8, 12, 10, 5, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock($1)`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM ai_usage_reservations WHERE expires_at <= NOW()`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT COALESCE((SELECT SUM(input_tokens + output_tokens) FROM ai_explanations`).
		WillReturnRows(sqlmock.NewRows([]string{"committed", "reserved"}).AddRow(1_000_000, 1_000_000))
	mock.ExpectExec(`INSERT INTO ai_usage_reservations`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.Reserve(context.Background(), Reservation{ID: "reservation-3", DiagnosisID: 7, ReservedTokens: 100, ExpiresAt: expires}, 0); err != nil {
		t.Fatalf("Reserve with disabled budget: %v", err)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryReservePropagatesStepFailures(t *testing.T) {
	expires := time.Date(2026, 8, 12, 10, 5, 0, 0, time.UTC)
	cases := []struct {
		name  string
		setup func(mock sqlmock.Sqlmock, failure error)
	}{
		{
			name: "advisory lock",
			setup: func(mock sqlmock.Sqlmock, failure error) {
				mock.ExpectExec(`SELECT pg_advisory_xact_lock($1)`).WillReturnError(failure)
			},
		},
		{
			name: "expired reservation cleanup",
			setup: func(mock sqlmock.Sqlmock, failure error) {
				mock.ExpectExec(`SELECT pg_advisory_xact_lock($1)`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`DELETE FROM ai_usage_reservations WHERE expires_at <= NOW()`).WillReturnError(failure)
			},
		},
		{
			name: "usage aggregate read",
			setup: func(mock sqlmock.Sqlmock, failure error) {
				mock.ExpectExec(`SELECT pg_advisory_xact_lock($1)`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`DELETE FROM ai_usage_reservations WHERE expires_at <= NOW()`).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(`SELECT COALESCE((SELECT SUM(input_tokens + output_tokens) FROM ai_explanations`).WillReturnError(failure)
			},
		},
		{
			name: "reservation insert",
			setup: func(mock sqlmock.Sqlmock, failure error) {
				mock.ExpectExec(`SELECT pg_advisory_xact_lock($1)`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`DELETE FROM ai_usage_reservations WHERE expires_at <= NOW()`).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(`SELECT COALESCE((SELECT SUM(input_tokens + output_tokens) FROM ai_explanations`).
					WillReturnRows(sqlmock.NewRows([]string{"committed", "reserved"}).AddRow(0, 0))
				mock.ExpectExec(`INSERT INTO ai_usage_reservations`).WillReturnError(failure)
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			gdb, mock := newMockGorm(t)
			repo := NewGormRepository(gdb)
			failure := errors.New("reserve step failed")

			mock.ExpectBegin()
			testCase.setup(mock, failure)
			mock.ExpectRollback()

			err := repo.Reserve(context.Background(), Reservation{ID: "reservation-x", DiagnosisID: 7, ReservedTokens: 10, ExpiresAt: expires}, 5000)
			if !errors.Is(err, failure) {
				t.Fatalf("Reserve() error = %v, want %v", err, failure)
			}
			mustExpectationsMet(t, mock)
		})
	}
}

func TestGormRepositoryReleaseDeletesReservation(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectExec(`DELETE FROM ai_usage_reservations WHERE id = $1`).
		WithArgs("reservation-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Release(context.Background(), "reservation-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryReleasePropagatesError(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	failure := errors.New("release failed")

	mock.ExpectExec(`DELETE FROM ai_usage_reservations WHERE id = $1`).WillReturnError(failure)

	if err := repo.Release(context.Background(), "reservation-1"); !errors.Is(err, failure) {
		t.Fatalf("Release() error = %v, want %v", err, failure)
	}
	mustExpectationsMet(t, mock)
}

// ---------------------------------------------------------------------------
// Save
// ---------------------------------------------------------------------------

func TestGormRepositorySavePersistsExplanationAndIdentity(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	created := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`INSERT INTO ai_explanations (diagnosis_id, actor_user_id, actor_name, provider, model, provider_response_id, summary, analysis, recommended_actions, citations, input_tokens, output_tokens) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CAST($9 AS JSONB), CAST($10 AS JSONB), $11, $12) RETURNING id, created_at`).
		WithArgs(int64(7), int64(3), "Operator", "responses-compatible", "gpt-test", "resp_1", "summary", "analysis",
			`[{"action":"restart","priority":"high","evidence_ids":["E1"]}]`,
			`[{"evidence_id":"E1","claim":"container waiting"}]`, 120, 45).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(42, created))

	item := Explanation{
		DiagnosisID: 7, Actor: ActorRef{ID: 3, Name: "Operator"},
		Provider: "responses-compatible", Model: "gpt-test", ProviderResponseID: "resp_1",
		Summary: "summary", Analysis: "analysis",
		RecommendedActions: []RecommendedAction{{Action: "restart", Priority: "high", EvidenceIDs: []string{"E1"}}},
		Citations:          []Citation{{EvidenceID: "E1", Claim: "container waiting"}},
		InputTokens:        120, OutputTokens: 45,
	}
	if err := repo.Save(context.Background(), &item); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if item.ID != 42 || !item.CreatedAt.Equal(created) {
		t.Fatalf("identity = id:%d created:%v", item.ID, item.CreatedAt)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositorySaveStoresNullActorAndEmptyCollections(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	created := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`INSERT INTO ai_explanations`).
		WithArgs(int64(7), nil, "", "nop", "deterministic", "", "summary", "analysis", "null", "null", 0, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, created))

	item := Explanation{DiagnosisID: 7, Provider: "nop", Model: "deterministic", Summary: "summary", Analysis: "analysis"}
	if err := repo.Save(context.Background(), &item); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if item.ID != 1 {
		t.Fatalf("id = %d, want 1", item.ID)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositorySavePropagatesError(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	failure := errors.New("insert failed")

	mock.ExpectQuery(`INSERT INTO ai_explanations`).WillReturnError(failure)

	item := Explanation{DiagnosisID: 7}
	if err := repo.Save(context.Background(), &item); !errors.Is(err, failure) {
		t.Fatalf("Save() error = %v, want %v", err, failure)
	}
	if item.ID != 0 {
		t.Fatalf("id = %d, want 0 on failure", item.ID)
	}
	mustExpectationsMet(t, mock)
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestGormRepositoryListMapsRowsAndPersonalFeedback(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	created := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	feedbackAt := time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`FROM ai_explanations e LEFT JOIN ai_explanation_feedback f ON f.explanation_id = e.id`).
		WithArgs(int64(42), int64(7)).
		WillReturnRows(sqlmock.NewRows(listColumns).
			AddRow(2, 7, 3, "Operator", "responses-compatible", "gpt-test", "resp_1", "s1", "a1",
				`[{"action":"restart","priority":"high","evidence_ids":["E1"]}]`,
				`[{"evidence_id":"E1","claim":"container waiting"}]`, 10, 20, created,
				3, 2, 1, 0, 9, "helpful", "clear", "Viewer", feedbackAt).
			AddRow(1, 7, nil, "System", "nop", "deterministic", "", "s0", "a0", "[]", "[]", 0, 0, nil,
				0, 0, 0, 0, nil, "", "", "", nil))

	items, err := repo.List(context.Background(), 7, 42)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}

	first := items[0]
	if first.ID != 2 || first.DiagnosisID != 7 || first.Actor.ID != 3 || first.Actor.Name != "Operator" {
		t.Fatalf("first item identity = %#v", first)
	}
	if first.Provider != "responses-compatible" || first.Model != "gpt-test" || first.ProviderResponseID != "resp_1" {
		t.Fatalf("first item provider = %#v", first)
	}
	if len(first.RecommendedActions) != 1 || first.RecommendedActions[0].Action != "restart" ||
		first.RecommendedActions[0].Priority != "high" || first.RecommendedActions[0].EvidenceIDs[0] != "E1" {
		t.Fatalf("recommended actions = %#v", first.RecommendedActions)
	}
	if len(first.Citations) != 1 || first.Citations[0].EvidenceID != "E1" || first.Citations[0].Claim != "container waiting" {
		t.Fatalf("citations = %#v", first.Citations)
	}
	if first.InputTokens != 10 || first.OutputTokens != 20 || !first.CreatedAt.Equal(created) {
		t.Fatalf("first item metrics = %#v", first)
	}
	if first.FeedbackSummary.Total != 3 || first.FeedbackSummary.Helpful != 2 ||
		first.FeedbackSummary.PartiallyHelpful != 1 || first.FeedbackSummary.NotHelpful != 0 ||
		first.FeedbackSummary.HelpfulRate != 2.0/3.0 {
		t.Fatalf("feedback summary = %#v", first.FeedbackSummary)
	}
	if first.MyFeedback == nil || first.MyFeedback.ID != 9 || first.MyFeedback.ExplanationID != 2 ||
		first.MyFeedback.Actor.ID != 42 || first.MyFeedback.Actor.Name != "Viewer" ||
		first.MyFeedback.Verdict != "helpful" || first.MyFeedback.Comment != "clear" ||
		!first.MyFeedback.CreatedAt.Equal(feedbackAt) {
		t.Fatalf("my feedback = %#v", first.MyFeedback)
	}

	second := items[1]
	if second.ID != 1 || second.Actor.ID != 0 || second.MyFeedback != nil {
		t.Fatalf("second item = %#v", second)
	}
	if !second.CreatedAt.IsZero() {
		t.Fatalf("second created at = %v, want zero", second.CreatedAt)
	}
	if len(second.RecommendedActions) != 0 || len(second.Citations) != 0 {
		t.Fatalf("second item collections = %#v", second)
	}
	if second.FeedbackSummary.Total != 0 || second.FeedbackSummary.HelpfulRate != 0 {
		t.Fatalf("second feedback summary = %#v", second.FeedbackSummary)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryListReturnsEmptySliceWhenNoRows(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`FROM ai_explanations e LEFT JOIN ai_explanation_feedback f`).
		WillReturnRows(sqlmock.NewRows(listColumns))

	items, err := repo.List(context.Background(), 7, 42)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("items = %#v, want empty non-nil slice", items)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryListPropagatesQueryError(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	failure := errors.New("list failed")

	mock.ExpectQuery(`FROM ai_explanations e LEFT JOIN ai_explanation_feedback f`).WillReturnError(failure)

	items, err := repo.List(context.Background(), 7, 42)
	if !errors.Is(err, failure) || items != nil {
		t.Fatalf("List() items=%#v error=%v, want nil/%v", items, err, failure)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryListRejectsMalformedJSONColumns(t *testing.T) {
	created := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		actions  string
		citation string
	}{
		{name: "recommended actions", actions: `{not-json`, citation: `[]`},
		{name: "citations", actions: `[]`, citation: `{not-json`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			gdb, mock := newMockGorm(t)
			repo := NewGormRepository(gdb)

			mock.ExpectQuery(`FROM ai_explanations e LEFT JOIN ai_explanation_feedback f`).
				WillReturnRows(sqlmock.NewRows(listColumns).
					AddRow(2, 7, 3, "Operator", "nop", "deterministic", "", "s1", "a1",
						testCase.actions, testCase.citation, 0, 0, created,
						0, 0, 0, 0, nil, "", "", "", nil))

			if _, err := repo.List(context.Background(), 7, 42); err == nil {
				t.Fatal("List() error = nil, want JSON decoding failure")
			}
			mustExpectationsMet(t, mock)
		})
	}
}

// ---------------------------------------------------------------------------
// AddFeedback / feedbackResult
// ---------------------------------------------------------------------------

func TestGormRepositoryAddFeedbackInsertsAndReturnsSummary(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	at := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	mock.ExpectExec(`INSERT INTO ai_explanation_feedback (explanation_id, actor_user_id, actor_name, verdict, comment) SELECT id, $1, $2, $3, $4 FROM ai_explanations WHERE id = $5 ON CONFLICT (explanation_id, actor_user_id) DO NOTHING`).
		WithArgs(int64(3), "Viewer", "helpful", "clear", int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`FROM ai_explanation_feedback mine JOIN ai_explanation_feedback all_feedback ON all_feedback.explanation_id = mine.explanation_id`).
		WithArgs(int64(8), int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "explanation_id", "actor_user_id", "actor_name", "verdict", "comment", "created_at", "total", "helpful", "partially_helpful", "not_helpful"}).
			AddRow(9, 8, 3, "Viewer", "helpful", "clear", at, 4, 3, 1, 0))

	result, err := repo.AddFeedback(context.Background(), 8, ActorRef{ID: 3, Name: "Viewer"}, "helpful", "clear")
	if err != nil {
		t.Fatalf("AddFeedback: %v", err)
	}
	if result.Feedback.ID != 9 || result.Feedback.ExplanationID != 8 || result.Feedback.Actor.ID != 3 ||
		result.Feedback.Actor.Name != "Viewer" || result.Feedback.Verdict != "helpful" ||
		result.Feedback.Comment != "clear" || !result.Feedback.CreatedAt.Equal(at) {
		t.Fatalf("feedback = %#v", result.Feedback)
	}
	if result.Summary.Total != 4 || result.Summary.Helpful != 3 || result.Summary.PartiallyHelpful != 1 ||
		result.Summary.NotHelpful != 0 || result.Summary.HelpfulRate != 0.75 {
		t.Fatalf("summary = %#v", result.Summary)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryAddFeedbackRejectsDuplicate(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectExec(`INSERT INTO ai_explanation_feedback`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT EXISTS(SELECT 1 FROM ai_explanations WHERE id = $1)`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	result, err := repo.AddFeedback(context.Background(), 8, ActorRef{ID: 3}, "helpful", "")
	if !errors.Is(err, ErrFeedbackExists) {
		t.Fatalf("AddFeedback() error = %v, want ErrFeedbackExists", err)
	}
	if result.Feedback.ID != 0 || result.Summary.Total != 0 {
		t.Fatalf("result = %#v, want zero value", result)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryAddFeedbackRejectsUnknownExplanation(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectExec(`INSERT INTO ai_explanation_feedback`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT EXISTS(SELECT 1 FROM ai_explanations WHERE id = $1)`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	if _, err := repo.AddFeedback(context.Background(), 8, ActorRef{ID: 3}, "helpful", ""); !errors.Is(err, ErrExplanationNotFound) {
		t.Fatalf("AddFeedback() error = %v, want ErrExplanationNotFound", err)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryAddFeedbackPropagatesFailures(t *testing.T) {
	t.Run("insert", func(t *testing.T) {
		gdb, mock := newMockGorm(t)
		repo := NewGormRepository(gdb)
		failure := errors.New("insert failed")

		mock.ExpectExec(`INSERT INTO ai_explanation_feedback`).WillReturnError(failure)

		if _, err := repo.AddFeedback(context.Background(), 8, ActorRef{ID: 3}, "helpful", ""); !errors.Is(err, failure) {
			t.Fatalf("AddFeedback() error = %v, want %v", err, failure)
		}
		mustExpectationsMet(t, mock)
	})

	t.Run("existence probe", func(t *testing.T) {
		gdb, mock := newMockGorm(t)
		repo := NewGormRepository(gdb)
		failure := errors.New("probe failed")

		mock.ExpectExec(`INSERT INTO ai_explanation_feedback`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS(SELECT 1 FROM ai_explanations WHERE id = $1)`).WillReturnError(failure)

		if _, err := repo.AddFeedback(context.Background(), 8, ActorRef{ID: 3}, "helpful", ""); !errors.Is(err, failure) {
			t.Fatalf("AddFeedback() error = %v, want %v", err, failure)
		}
		mustExpectationsMet(t, mock)
	})
}

func TestGormRepositoryFeedbackResultPropagatesError(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	failure := errors.New("feedback lookup failed")

	mock.ExpectQuery(`FROM ai_explanation_feedback mine JOIN ai_explanation_feedback all_feedback`).WillReturnError(failure)

	result, err := repo.feedbackResult(context.Background(), 8, 3)
	if !errors.Is(err, failure) {
		t.Fatalf("feedbackResult() error = %v, want %v", err, failure)
	}
	if result.Feedback.ID != 0 || result.Summary.Total != 0 {
		t.Fatalf("result = %#v, want zero value", result)
	}
	mustExpectationsMet(t, mock)
}

// ---------------------------------------------------------------------------
// Quality
// ---------------------------------------------------------------------------

func TestGormRepositoryQualityAggregatesTotalsAndModels(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`COUNT(DISTINCT actor_user_id)::int AS contributors FROM ai_explanation_feedback`).
		WillReturnRows(sqlmock.NewRows([]string{"total_feedback", "helpful", "partially_helpful", "not_helpful", "explanations_with_feedback", "contributors"}).
			AddRow(10, 6, 3, 1, 5, 3))
	mock.ExpectQuery(`FROM ai_explanation_feedback f JOIN ai_explanations e ON e.id = f.explanation_id GROUP BY e.model`).
		WillReturnRows(sqlmock.NewRows([]string{"model", "total_feedback", "helpful", "partially_helpful", "not_helpful"}).
			AddRow("gpt-test", 7, 5, 2, 0).
			AddRow("deterministic", 3, 1, 1, 1))

	quality, err := repo.Quality(context.Background())
	if err != nil {
		t.Fatalf("Quality: %v", err)
	}
	if quality.TotalFeedback != 10 || quality.Helpful != 6 || quality.PartiallyHelpful != 3 || quality.NotHelpful != 1 {
		t.Fatalf("quality totals = %#v", quality)
	}
	if quality.HelpfulRate != 0.6 || quality.ExplanationsWithFeedback != 5 || quality.Contributors != 3 {
		t.Fatalf("quality rates = %#v", quality)
	}
	if len(quality.ByModel) != 2 {
		t.Fatalf("by model = %#v", quality.ByModel)
	}
	if quality.ByModel[0].Model != "gpt-test" || quality.ByModel[0].TotalFeedback != 7 ||
		quality.ByModel[0].Helpful != 5 || quality.ByModel[0].HelpfulRate != 5.0/7.0 {
		t.Fatalf("first model = %#v", quality.ByModel[0])
	}
	if quality.ByModel[1].Model != "deterministic" || quality.ByModel[1].NotHelpful != 1 || quality.ByModel[1].HelpfulRate != 1.0/3.0 {
		t.Fatalf("second model = %#v", quality.ByModel[1])
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryQualityWithoutFeedbackReturnsZeroRates(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`COUNT(DISTINCT actor_user_id)::int AS contributors FROM ai_explanation_feedback`).
		WillReturnRows(sqlmock.NewRows([]string{"total_feedback", "helpful", "partially_helpful", "not_helpful", "explanations_with_feedback", "contributors"}).
			AddRow(0, 0, 0, 0, 0, 0))
	mock.ExpectQuery(`FROM ai_explanation_feedback f JOIN ai_explanations e ON e.id = f.explanation_id GROUP BY e.model`).
		WillReturnRows(sqlmock.NewRows([]string{"model", "total_feedback", "helpful", "partially_helpful", "not_helpful"}))

	quality, err := repo.Quality(context.Background())
	if err != nil {
		t.Fatalf("Quality: %v", err)
	}
	if quality.HelpfulRate != 0 || len(quality.ByModel) != 0 || quality.ByModel == nil {
		t.Fatalf("quality = %#v", quality)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryQualityPropagatesErrors(t *testing.T) {
	t.Run("totals", func(t *testing.T) {
		gdb, mock := newMockGorm(t)
		repo := NewGormRepository(gdb)
		failure := errors.New("totals failed")

		mock.ExpectQuery(`COUNT(DISTINCT actor_user_id)::int AS contributors FROM ai_explanation_feedback`).WillReturnError(failure)

		if _, err := repo.Quality(context.Background()); !errors.Is(err, failure) {
			t.Fatalf("Quality() error = %v, want %v", err, failure)
		}
		mustExpectationsMet(t, mock)
	})

	t.Run("by model", func(t *testing.T) {
		gdb, mock := newMockGorm(t)
		repo := NewGormRepository(gdb)
		failure := errors.New("by model failed")

		mock.ExpectQuery(`COUNT(DISTINCT actor_user_id)::int AS contributors FROM ai_explanation_feedback`).
			WillReturnRows(sqlmock.NewRows([]string{"total_feedback", "helpful", "partially_helpful", "not_helpful", "explanations_with_feedback", "contributors"}).
				AddRow(0, 0, 0, 0, 0, 0))
		mock.ExpectQuery(`FROM ai_explanation_feedback f JOIN ai_explanations e ON e.id = f.explanation_id GROUP BY e.model`).WillReturnError(failure)

		if _, err := repo.Quality(context.Background()); !errors.Is(err, failure) {
			t.Fatalf("Quality() error = %v, want %v", err, failure)
		}
		mustExpectationsMet(t, mock)
	})
}

// ---------------------------------------------------------------------------
// Coverage
// ---------------------------------------------------------------------------

func TestGormRepositoryCoverageAggregatesSnapshot(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`COUNT(*) FILTER (WHERE provider = 'nop')::int AS deterministic_count FROM ai_explanations`).
		WillReturnRows(sqlmock.NewRows([]string{"total_explanations", "explained_diagnoses", "with_citations", "deterministic_count"}).
			AddRow(8, 5, 6, 2))
	mock.ExpectQuery(`COUNT(DISTINCT actor_user_id)::int AS contributors FROM ai_explanation_feedback`).
		WillReturnRows(sqlmock.NewRows([]string{"total_feedback", "helpful", "partially_helpful", "not_helpful", "explanations_with_feedback", "contributors"}).
			AddRow(10, 6, 3, 1, 5, 3))
	mock.ExpectQuery(`FROM ai_explanation_feedback f JOIN ai_explanations e ON e.id = f.explanation_id GROUP BY e.model`).
		WillReturnRows(sqlmock.NewRows([]string{"model", "total_feedback", "helpful", "partially_helpful", "not_helpful"}).
			AddRow("gpt-test", 10, 6, 3, 1))

	coverage, err := repo.Coverage(context.Background())
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if coverage.TotalExplanations != 8 || coverage.ExplainedDiagnoses != 5 ||
		coverage.WithCitations != 6 || coverage.DeterministicCount != 2 {
		t.Fatalf("coverage counts = %#v", coverage)
	}
	if coverage.CitationRate != 0.75 || coverage.DeterministicRate != 0.25 {
		t.Fatalf("coverage rates = %#v", coverage)
	}
	if coverage.Quality.TotalFeedback != 10 || coverage.Quality.HelpfulRate != 0.6 {
		t.Fatalf("coverage quality baseline = %#v", coverage.Quality)
	}
	if coverage.WindowNote == "" {
		t.Fatal("window note must be populated")
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryCoverageWithoutExplanationsUsesZeroRates(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`FROM ai_explanations`).
		WillReturnRows(sqlmock.NewRows([]string{"total_explanations", "explained_diagnoses", "with_citations", "deterministic_count"}).
			AddRow(0, 0, 0, 0))
	mock.ExpectQuery(`COUNT(DISTINCT actor_user_id)::int AS contributors FROM ai_explanation_feedback`).
		WillReturnRows(sqlmock.NewRows([]string{"total_feedback", "helpful", "partially_helpful", "not_helpful", "explanations_with_feedback", "contributors"}).
			AddRow(0, 0, 0, 0, 0, 0))
	mock.ExpectQuery(`GROUP BY e.model`).
		WillReturnRows(sqlmock.NewRows([]string{"model", "total_feedback", "helpful", "partially_helpful", "not_helpful"}))

	coverage, err := repo.Coverage(context.Background())
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if coverage.CitationRate != 0 || coverage.DeterministicRate != 0 {
		t.Fatalf("coverage rates = %#v", coverage)
	}
	mustExpectationsMet(t, mock)
}

func TestGormRepositoryCoveragePropagatesErrors(t *testing.T) {
	t.Run("statistics", func(t *testing.T) {
		gdb, mock := newMockGorm(t)
		repo := NewGormRepository(gdb)
		failure := errors.New("stats failed")

		mock.ExpectQuery(`FROM ai_explanations`).WillReturnError(failure)

		if _, err := repo.Coverage(context.Background()); !errors.Is(err, failure) {
			t.Fatalf("Coverage() error = %v, want %v", err, failure)
		}
		mustExpectationsMet(t, mock)
	})

	t.Run("quality baseline", func(t *testing.T) {
		gdb, mock := newMockGorm(t)
		repo := NewGormRepository(gdb)
		failure := errors.New("quality failed")

		mock.ExpectQuery(`FROM ai_explanations`).
			WillReturnRows(sqlmock.NewRows([]string{"total_explanations", "explained_diagnoses", "with_citations", "deterministic_count"}).
				AddRow(1, 1, 1, 0))
		mock.ExpectQuery(`COUNT(DISTINCT actor_user_id)::int AS contributors FROM ai_explanation_feedback`).WillReturnError(failure)

		if _, err := repo.Coverage(context.Background()); !errors.Is(err, failure) {
			t.Fatalf("Coverage() error = %v, want %v", err, failure)
		}
		mustExpectationsMet(t, mock)
	})
}
