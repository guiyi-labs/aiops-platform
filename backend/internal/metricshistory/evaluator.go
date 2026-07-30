package metricshistory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	OperatorGreaterThanOrEqual = "gte"
	OperatorLessThanOrEqual    = "lte"

	EvaluationStateInsufficientData = "insufficient_data"
	EvaluationStateFiring           = "firing"
	EvaluationStateNormal           = "normal"

	defaultMinimumPoints = 2
	minEvaluationSeconds = 60
	maxEvaluationSeconds = 86400
	minEvaluationPoints  = 2
	maxEvaluationPoints  = 1440

	maxSustainedWindows = 64
)

var ErrInvalidEvaluation = errors.New("metrics history evaluation is invalid")

type EvaluationRule struct {
	Operator      string `json:"operator"`
	Threshold     int64  `json:"threshold"`
	ForSeconds    int    `json:"for_seconds"`
	MinimumPoints int    `json:"minimum_points"`
}

type SustainedWindow struct {
	StartCollectedAt time.Time `json:"start_collected_at"`
	EndCollectedAt   time.Time `json:"end_collected_at"`
	BreachingPoints  int       `json:"breaching_points"`
	SpanSeconds      int64     `json:"span_seconds"`
}

type EvaluationQuery struct {
	SeriesQuery
	EvaluationRule
}

type EvaluationResponse struct {
	Series              Series            `json:"series"`
	From                time.Time         `json:"from"`
	To                  time.Time         `json:"to"`
	Coverage            QueryCoverage     `json:"coverage"`
	State               string            `json:"state"`
	Operator            string            `json:"operator"`
	Threshold           int64             `json:"threshold"`
	ForSeconds          int               `json:"for_seconds"`
	MinimumPoints       int               `json:"minimum_points"`
	PointsEvaluated     int               `json:"points_evaluated"`
	BreachingPoints     int               `json:"breaching_points"`
	ObservedSpanSeconds int64             `json:"observed_span_seconds"`
	SustainedWindows    []SustainedWindow `json:"sustained_windows"`
	LatestFiringWindow  *SustainedWindow  `json:"latest_firing_window,omitempty"`
}

func (s *Service) Evaluate(ctx context.Context, query EvaluationQuery) (EvaluationResponse, error) {
	rule, err := normalizeEvaluationRule(query.EvaluationRule)
	if err != nil {
		return EvaluationResponse{}, err
	}
	series, err := s.Query(ctx, query.SeriesQuery)
	if err != nil {
		return EvaluationResponse{}, err
	}
	return EvaluateWindow(series, rule)
}

// EvaluateWindow is pure: it sorts a copy and never mutates the input series.
func EvaluateWindow(series SeriesResponse, rule EvaluationRule) (EvaluationResponse, error) {
	rule, err := normalizeEvaluationRule(rule)
	if err != nil {
		return EvaluationResponse{}, err
	}
	points := append([]Point(nil), series.Points...)
	sort.SliceStable(points, func(i, j int) bool {
		if points[i].CollectedAt.Equal(points[j].CollectedAt) {
			return points[i].SourceTimestamp.Before(points[j].SourceTimestamp)
		}
		return points[i].CollectedAt.Before(points[j].CollectedAt)
	})
	response := EvaluationResponse{
		Series: series.Series, From: series.From, To: series.To, Coverage: series.Coverage,
		State: EvaluationStateNormal, Operator: rule.Operator, Threshold: rule.Threshold,
		ForSeconds: rule.ForSeconds, MinimumPoints: rule.MinimumPoints, PointsEvaluated: len(points),
	}
	sustainedWindows := findSustainedWindows(points, rule)
	response.SustainedWindows = sustainedWindows
	if len(sustainedWindows) > 0 {
		latest := sustainedWindows[len(sustainedWindows)-1]
		response.LatestFiringWindow = &latest
		response.BreachingPoints = latest.BreachingPoints
		response.ObservedSpanSeconds = latest.SpanSeconds
	}
	coverage := series.Coverage
	if series.Truncated || coverage.Missing > 0 || coverage.Unavailable > 0 || coverage.TimedOut > 0 || coverage.Failed > 0 || len(points) < rule.MinimumPoints {
		response.State = EvaluationStateInsufficientData
	} else if response.LatestFiringWindow != nil {
		response.State = EvaluationStateFiring
	}
	return response, nil
}

// findSustainedWindows scans the entire sorted series for all breach windows
// that satisfy the minimum points and minimum duration rule. It returns at most
// maxSustainedWindows, keeping the most recent ones.
func findSustainedWindows(points []Point, rule EvaluationRule) []SustainedWindow {
	if len(points) == 0 {
		return nil
	}
	var windows []SustainedWindow
	var currentBreaches int
	var currentStart int
	inBreach := false
	for i := 0; i < len(points); i++ {
		if rule.breaches(points[i].Value) {
			if !inBreach {
				inBreach = true
				currentStart = i
				currentBreaches = 1
			} else {
				currentBreaches++
			}
		} else if inBreach {
			inBreach = false
			window, ok := buildSustainedWindow(points, currentStart, i-1, currentBreaches, rule)
			if ok {
				windows = append(windows, window)
			}
			currentBreaches = 0
		}
	}
	if inBreach {
		window, ok := buildSustainedWindow(points, currentStart, len(points)-1, currentBreaches, rule)
		if ok {
			windows = append(windows, window)
		}
	}
	if len(windows) > maxSustainedWindows {
		windows = windows[len(windows)-maxSustainedWindows:]
	}
	return windows
}

func buildSustainedWindow(points []Point, startIdx, endIdx, breachCount int, rule EvaluationRule) (SustainedWindow, bool) {
	if breachCount < rule.MinimumPoints {
		return SustainedWindow{}, false
	}
	first := points[startIdx].CollectedAt
	last := points[endIdx].CollectedAt
	span := int64(last.Sub(first) / time.Second)
	if span < int64(rule.ForSeconds) {
		return SustainedWindow{}, false
	}
	return SustainedWindow{
		StartCollectedAt: first, EndCollectedAt: last,
		BreachingPoints: breachCount, SpanSeconds: span,
	}, true
}

func normalizeEvaluationRule(rule EvaluationRule) (EvaluationRule, error) {
	rule.Operator = strings.TrimSpace(rule.Operator)
	if rule.MinimumPoints == 0 {
		rule.MinimumPoints = defaultMinimumPoints
	}
	if (rule.Operator != OperatorGreaterThanOrEqual && rule.Operator != OperatorLessThanOrEqual) ||
		rule.Threshold < 0 || rule.ForSeconds < minEvaluationSeconds || rule.ForSeconds > maxEvaluationSeconds ||
		rule.MinimumPoints < minEvaluationPoints || rule.MinimumPoints > maxEvaluationPoints {
		return EvaluationRule{}, ErrInvalidEvaluation
	}
	return rule, nil
}

func (r EvaluationRule) breaches(value int64) bool {
	if r.Operator == OperatorGreaterThanOrEqual {
		return value >= r.Threshold
	}
	return value <= r.Threshold
}
