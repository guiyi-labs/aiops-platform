package alert

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

type SchedulerConfig struct {
	Enabled           bool
	PollInterval      time.Duration
	ClaimBatch        int
	WorkerConcurrency int
	EvaluationTimeout time.Duration
	ClaimLease        time.Duration
	MinEvalInterval   time.Duration
}

type Scheduler struct {
	config  SchedulerConfig
	service *Service
	repo    Repository
	logger  *zap.Logger
	now     func() time.Time
}

func NewScheduler(config SchedulerConfig, service *Service, repo Repository, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		config:  config,
		service: service,
		repo:    repo,
		logger:  logger,
		now:     time.Now,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info("alert scheduler is disabled")
		<-ctx.Done()
		return
	}
	s.logger.Info("alert scheduler started",
		zap.Duration("poll_interval", s.config.PollInterval),
		zap.Int("claim_batch", s.config.ClaimBatch),
		zap.Int("worker_concurrency", s.config.WorkerConcurrency),
	)

	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("alert scheduler stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	now := s.now()
	rules, err := s.repo.ClaimDueRules(ctx, now, s.config.ClaimBatch, s.config.ClaimLease)
	if err != nil {
		s.logger.Error("alert scheduler claim failed", zap.Error(err))
		return
	}
	if len(rules) == 0 {
		return
	}

	sem := make(chan struct{}, s.config.WorkerConcurrency)
	var wg sync.WaitGroup

	for _, rule := range rules {
		wg.Add(1)
		sem <- struct{}{}
		go func(r Rule) {
			defer wg.Done()
			defer func() { <-sem }()

			evalCtx, cancel := context.WithTimeout(ctx, s.config.EvaluationTimeout)
			defer cancel()

			if err := s.service.EvaluateRule(evalCtx, r); err != nil {
				s.logger.Error("alert rule evaluation failed",
					zap.Int64("rule_id", r.ID),
					zap.String("rule_name", r.DisplayName),
					zap.Error(err),
				)
			}
		}(rule)
	}

	wg.Wait()
}
