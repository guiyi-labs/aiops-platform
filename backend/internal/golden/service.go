package golden

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Service is the M56 quality-report service. It wraps the replay runner
// and report storage, exposing synchronous report reads and async replay
// execution with in-memory task tracking.
type Service struct {
	mu      sync.Mutex
	runner  *ReplayRunner
	storage ReportStorage
	logger  *zap.Logger
	tasks   *replayTaskTracker
}

// NewService returns a Service bound to the given engine contracts and
// report storage. The contracts are captured at construction time so
// the runner sees a consistent snapshot of engine versions.
func NewService(contracts EngineContracts, storage ReportStorage, logger *zap.Logger) *Service {
	return &Service{
		runner:  NewReplayRunner(contracts),
		storage: storage,
		logger:  logger,
		tasks:   newReplayTaskTracker(),
	}
}

// GetLatestReport returns the most recently saved quality report. Returns
// ErrNoReport if no report has been generated yet.
func (s *Service) GetLatestReport() (QualityReport, error) {
	return s.storage.LoadLatest()
}

// ReplayTaskView is the API projection of an async replay task.
type ReplayTaskView struct {
	ID        string           `json:"id"`
	Status    ReplayTaskStatus `json:"status"`
	StartedAt time.Time        `json:"started_at"`
	EndedAt   time.Time        `json:"ended_at,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// RunReplay triggers an async golden dataset replay. It returns the task
// ID immediately; the replay executes in a background goroutine. The
// caller can poll GetTask(id) for completion status.
func (s *Service) RunReplay(ctx context.Context) (string, error) {
	taskID := generateTaskID()
	task := &ReplayTask{
		ID:        taskID,
		Status:    ReplayTaskRunning,
		StartedAt: NowFunc(),
	}
	s.tasks.set(task)

	go s.executeReplay(taskID)

	return taskID, nil
}

// GetTask returns the current status of an async replay task. Returns
// false if the task ID is unknown (e.g. expired or never created).
func (s *Service) GetTask(id string) (ReplayTaskView, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks.get(id)
	if !ok {
		return ReplayTaskView{}, false
	}
	return ReplayTaskView{
		ID:        task.ID,
		Status:    task.Status,
		StartedAt: task.StartedAt,
		EndedAt:   task.EndedAt,
		Error:     task.Error,
	}, true
}

func (s *Service) executeReplay(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	s.mu.Lock()
	task, _ := s.tasks.get(taskID)
	s.mu.Unlock()

	dataset := DefaultDataset()
	results, err := s.runner.Run(ctx, dataset)
	if err != nil {
		s.mu.Lock()
		task.Status = ReplayTaskFailed
		task.EndedAt = NowFunc()
		task.Error = fmt.Sprintf("replay runner failed: %v", err)
		s.tasks.set(task)
		s.mu.Unlock()
		s.logger.Error("golden replay failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}

	// Load the previous baseline report (if any) to compare before/after.
	baseline, baselineErr := s.storage.LoadLatest()
	var baselineMap map[ScenarioID]ScenarioQuality
	if baselineErr == nil {
		baselineMap = make(map[ScenarioID]ScenarioQuality, len(baseline.ScenarioResults))
		for _, sq := range baseline.ScenarioResults {
			baselineMap[sq.ScenarioID] = sq
		}
	}

	// Build per-scenario quality results.
	scenarioQualities := make([]ScenarioQuality, 0, len(results))
	for _, res := range results {
		var baselineSQ *ScenarioQuality
		if sq, ok := baselineMap[res.ScenarioID]; ok {
			baselineSQ = &sq
		}
		scenarioQualities = append(scenarioQualities, BuildScenarioQuality(res, baselineSQ))
	}

	report := QualityReport{
		ReportVersion:        ReportVersion,
		DatasetVersionBefore: dataset.Version,
		DatasetVersionAfter:  dataset.Version,
		EngineVersionsBefore: s.runner.contracts.Versions,
		EngineVersionsAfter:  s.runner.contracts.Versions,
		ScenarioResults:      scenarioQualities,
		Summary:              Summarize(scenarioQualities),
		GeneratedAt:          NowFunc(),
	}

	if baselineErr == nil {
		report.DatasetVersionBefore = baseline.DatasetVersionAfter
		report.EngineVersionsBefore = baseline.EngineVersionsAfter
	}

	if err := s.storage.Save(report); err != nil {
		s.mu.Lock()
		task.Status = ReplayTaskFailed
		task.EndedAt = NowFunc()
		task.Error = fmt.Sprintf("save quality report failed: %v", err)
		s.tasks.set(task)
		s.mu.Unlock()
		s.logger.Error("golden replay save failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}

	s.mu.Lock()
	task.Status = ReplayTaskSucceeded
	task.EndedAt = NowFunc()
	task.Report = &report
	s.tasks.set(task)
	s.mu.Unlock()
	s.logger.Info("golden replay succeeded",
		zap.String("task_id", taskID),
		zap.Int("scenarios", len(scenarioQualities)),
		zap.Int("passed", report.Summary.PassedAfter),
		zap.Int("regressed", report.Summary.Regressed),
	)
}

func generateTaskID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "qr-" + hex.EncodeToString(b)
}
