package aiexplain

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type Repository interface {
	Save(context.Context, *Explanation) error
	List(context.Context, int64, int64) ([]Explanation, error)
	AddFeedback(context.Context, int64, ActorRef, string, string) (FeedbackResult, error)
	Quality(context.Context) (QualitySummary, error)
	Usage(context.Context) (Usage, error)
	Reserve(context.Context, Reservation, int) error
	Release(context.Context, string) error
}

func (r *GormRepository) Usage(ctx context.Context) (Usage, error) {
	type stored struct {
		UsedTokensToday  int
		ReservedTokens   int
		ExplanationCount int
		LastSuccessAt    sql.NullTime
	}
	var value stored
	if err := r.db.WithContext(ctx).Raw(`SELECT
		COALESCE((SELECT SUM(input_tokens + output_tokens) FROM ai_explanations WHERE created_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'), 0) AS used_tokens_today,
		COALESCE((SELECT SUM(reserved_tokens) FROM ai_usage_reservations WHERE expires_at > NOW()), 0) AS reserved_tokens,
		(SELECT COUNT(*) FROM ai_explanations WHERE created_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC') AS explanation_count,
		(SELECT MAX(created_at) FROM ai_explanations) AS last_success_at`).Scan(&value).Error; err != nil {
		return Usage{}, err
	}
	usage := Usage{UsedTokensToday: value.UsedTokensToday, ReservedTokens: value.ReservedTokens, ExplanationCount: value.ExplanationCount}
	if value.LastSuccessAt.Valid {
		usage.LastSuccessAt = &value.LastSuccessAt.Time
	}
	return usage, nil
}

func (r *GormRepository) Reserve(ctx context.Context, reservation Reservation, dailyBudget int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, int64(741009)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM ai_usage_reservations WHERE expires_at <= NOW()`).Error; err != nil {
			return err
		}
		var committed, reserved int
		if err := tx.Raw(`SELECT
			COALESCE((SELECT SUM(input_tokens + output_tokens) FROM ai_explanations WHERE created_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'), 0),
			COALESCE((SELECT SUM(reserved_tokens) FROM ai_usage_reservations), 0)`).Row().Scan(&committed, &reserved); err != nil {
			return err
		}
		if dailyBudget > 0 && committed+reserved+reservation.ReservedTokens > dailyBudget {
			return ErrBudgetExceeded
		}
		return tx.Exec(`INSERT INTO ai_usage_reservations (id, diagnosis_id, reserved_tokens, expires_at) VALUES (?, ?, ?, ?)`, reservation.ID, reservation.DiagnosisID, reservation.ReservedTokens, reservation.ExpiresAt).Error
	})
}

func (r *GormRepository) Release(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Exec(`DELETE FROM ai_usage_reservations WHERE id = ?`, id).Error
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Save(ctx context.Context, item *Explanation) error {
	actions, _ := json.Marshal(item.RecommendedActions)
	citations, _ := json.Marshal(item.Citations)
	return r.db.WithContext(ctx).Raw(`INSERT INTO ai_explanations
		(diagnosis_id, actor_user_id, actor_name, provider, model, provider_response_id, summary, analysis, recommended_actions, citations, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS JSONB), CAST(? AS JSONB), ?, ?)
		RETURNING id, created_at`, item.DiagnosisID, nullableID(item.Actor.ID), item.Actor.Name, item.Provider, item.Model, item.ProviderResponseID, item.Summary, item.Analysis, string(actions), string(citations), item.InputTokens, item.OutputTokens).Row().Scan(&item.ID, &item.CreatedAt)
}

func (r *GormRepository) List(ctx context.Context, diagnosisID, actorID int64) ([]Explanation, error) {
	type stored struct {
		ID, DiagnosisID                                                              int64
		ActorUserID                                                                  sql.NullInt64
		ActorName, Provider, Model                                                   string
		ProviderResponseID                                                           string
		Summary, Analysis                                                            string
		RecommendedActions, Citations                                                string
		InputTokens, OutputTokens                                                    int
		FeedbackTotal, FeedbackHelpful, FeedbackPartiallyHelpful, FeedbackNotHelpful int
		MyFeedbackID                                                                 sql.NullInt64
		MyFeedbackVerdict, MyFeedbackComment, MyFeedbackActorName                    string
		MyFeedbackCreatedAt                                                          sql.NullTime
		CreatedAt                                                                    sql.NullTime
	}
	var rows []stored
	if err := r.db.WithContext(ctx).Raw(`SELECT e.id, e.diagnosis_id, e.actor_user_id, e.actor_name, e.provider, e.model, e.provider_response_id,
		e.summary, e.analysis, e.recommended_actions::text AS recommended_actions, e.citations::text AS citations, e.input_tokens, e.output_tokens, e.created_at,
		COUNT(f.id)::int AS feedback_total,
		COUNT(f.id) FILTER (WHERE f.verdict = 'helpful')::int AS feedback_helpful,
		COUNT(f.id) FILTER (WHERE f.verdict = 'partially_helpful')::int AS feedback_partially_helpful,
		COUNT(f.id) FILTER (WHERE f.verdict = 'not_helpful')::int AS feedback_not_helpful,
		mine.id AS my_feedback_id, mine.verdict AS my_feedback_verdict, mine.comment AS my_feedback_comment,
		mine.actor_name AS my_feedback_actor_name, mine.created_at AS my_feedback_created_at
		FROM ai_explanations e
		LEFT JOIN ai_explanation_feedback f ON f.explanation_id = e.id
		LEFT JOIN ai_explanation_feedback mine ON mine.explanation_id = e.id AND mine.actor_user_id = ?
		WHERE e.diagnosis_id = ?
		GROUP BY e.id, mine.id
		ORDER BY e.created_at DESC, e.id DESC LIMIT 20`, actorID, diagnosisID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]Explanation, 0, len(rows))
	for _, row := range rows {
		item := Explanation{ID: row.ID, DiagnosisID: row.DiagnosisID, Actor: ActorRef{ID: row.ActorUserID.Int64, Name: row.ActorName}, Provider: row.Provider, Model: row.Model, ProviderResponseID: row.ProviderResponseID, Summary: row.Summary, Analysis: row.Analysis, InputTokens: row.InputTokens, OutputTokens: row.OutputTokens,
			FeedbackSummary: feedbackSummary(row.FeedbackTotal, row.FeedbackHelpful, row.FeedbackPartiallyHelpful, row.FeedbackNotHelpful)}
		if row.CreatedAt.Valid {
			item.CreatedAt = row.CreatedAt.Time
		}
		if err := json.Unmarshal([]byte(row.RecommendedActions), &item.RecommendedActions); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(row.Citations), &item.Citations); err != nil {
			return nil, err
		}
		if row.MyFeedbackID.Valid {
			item.MyFeedback = &Feedback{ID: row.MyFeedbackID.Int64, ExplanationID: row.ID, Actor: ActorRef{ID: actorID, Name: row.MyFeedbackActorName}, Verdict: row.MyFeedbackVerdict, Comment: row.MyFeedbackComment, CreatedAt: row.MyFeedbackCreatedAt.Time}
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *GormRepository) AddFeedback(ctx context.Context, explanationID int64, actor ActorRef, verdict, comment string) (FeedbackResult, error) {
	result := r.db.WithContext(ctx).Exec(`INSERT INTO ai_explanation_feedback (explanation_id, actor_user_id, actor_name, verdict, comment)
		SELECT id, ?, ?, ?, ? FROM ai_explanations WHERE id = ?
		ON CONFLICT (explanation_id, actor_user_id) DO NOTHING`, actor.ID, actor.Name, verdict, comment, explanationID)
	if result.Error != nil {
		return FeedbackResult{}, result.Error
	}
	if result.RowsAffected == 0 {
		var exists bool
		if err := r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM ai_explanations WHERE id = ?)`, explanationID).Row().Scan(&exists); err != nil {
			return FeedbackResult{}, err
		}
		if !exists {
			return FeedbackResult{}, ErrExplanationNotFound
		}
		return FeedbackResult{}, ErrFeedbackExists
	}
	return r.feedbackResult(ctx, explanationID, actor.ID)
}

func (r *GormRepository) feedbackResult(ctx context.Context, explanationID, actorID int64) (FeedbackResult, error) {
	type stored struct {
		ID, ExplanationID, ActorUserID               int64
		ActorName, Verdict, Comment                  string
		CreatedAt                                    time.Time
		Total, Helpful, PartiallyHelpful, NotHelpful int
	}
	var value stored
	err := r.db.WithContext(ctx).Raw(`SELECT mine.id, mine.explanation_id, mine.actor_user_id, mine.actor_name, mine.verdict, mine.comment, mine.created_at,
		COUNT(all_feedback.id)::int AS total,
		COUNT(all_feedback.id) FILTER (WHERE all_feedback.verdict = 'helpful')::int AS helpful,
		COUNT(all_feedback.id) FILTER (WHERE all_feedback.verdict = 'partially_helpful')::int AS partially_helpful,
		COUNT(all_feedback.id) FILTER (WHERE all_feedback.verdict = 'not_helpful')::int AS not_helpful
		FROM ai_explanation_feedback mine
		JOIN ai_explanation_feedback all_feedback ON all_feedback.explanation_id = mine.explanation_id
		WHERE mine.explanation_id = ? AND mine.actor_user_id = ?
		GROUP BY mine.id`, explanationID, actorID).Scan(&value).Error
	if err != nil {
		return FeedbackResult{}, err
	}
	return FeedbackResult{Feedback: Feedback{ID: value.ID, ExplanationID: value.ExplanationID, Actor: ActorRef{ID: value.ActorUserID, Name: value.ActorName}, Verdict: value.Verdict, Comment: value.Comment, CreatedAt: value.CreatedAt}, Summary: feedbackSummary(value.Total, value.Helpful, value.PartiallyHelpful, value.NotHelpful)}, nil
}

func (r *GormRepository) Quality(ctx context.Context) (QualitySummary, error) {
	type stored struct {
		Model                                                string
		TotalFeedback, Helpful, PartiallyHelpful, NotHelpful int
		ExplanationsWithFeedback, Contributors               int
	}
	var total stored
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*)::int AS total_feedback,
		COUNT(*) FILTER (WHERE verdict = 'helpful')::int AS helpful,
		COUNT(*) FILTER (WHERE verdict = 'partially_helpful')::int AS partially_helpful,
		COUNT(*) FILTER (WHERE verdict = 'not_helpful')::int AS not_helpful,
		COUNT(DISTINCT explanation_id)::int AS explanations_with_feedback,
		COUNT(DISTINCT actor_user_id)::int AS contributors FROM ai_explanation_feedback`).Scan(&total).Error; err != nil {
		return QualitySummary{}, err
	}
	var models []stored
	if err := r.db.WithContext(ctx).Raw(`SELECT e.model, COUNT(f.id)::int AS total_feedback,
		COUNT(f.id) FILTER (WHERE f.verdict = 'helpful')::int AS helpful,
		COUNT(f.id) FILTER (WHERE f.verdict = 'partially_helpful')::int AS partially_helpful,
		COUNT(f.id) FILTER (WHERE f.verdict = 'not_helpful')::int AS not_helpful
		FROM ai_explanation_feedback f JOIN ai_explanations e ON e.id = f.explanation_id
		GROUP BY e.model ORDER BY COUNT(f.id) DESC, e.model`).Scan(&models).Error; err != nil {
		return QualitySummary{}, err
	}
	quality := QualitySummary{TotalFeedback: total.TotalFeedback, Helpful: total.Helpful, PartiallyHelpful: total.PartiallyHelpful, NotHelpful: total.NotHelpful, HelpfulRate: helpfulRate(total.TotalFeedback, total.Helpful), ExplanationsWithFeedback: total.ExplanationsWithFeedback, Contributors: total.Contributors, ByModel: make([]ModelQualitySummary, 0, len(models))}
	for _, model := range models {
		quality.ByModel = append(quality.ByModel, ModelQualitySummary{Model: model.Model, TotalFeedback: model.TotalFeedback, Helpful: model.Helpful, PartiallyHelpful: model.PartiallyHelpful, NotHelpful: model.NotHelpful, HelpfulRate: helpfulRate(model.TotalFeedback, model.Helpful)})
	}
	return quality, nil
}

func feedbackSummary(total, helpful, partiallyHelpful, notHelpful int) FeedbackSummary {
	return FeedbackSummary{Total: total, Helpful: helpful, PartiallyHelpful: partiallyHelpful, NotHelpful: notHelpful, HelpfulRate: helpfulRate(total, helpful)}
}

func helpfulRate(total, helpful int) float64 {
	if total == 0 {
		return 0
	}
	return float64(helpful) / float64(total)
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
