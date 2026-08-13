package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const maximumRetryDelay = 15 * time.Minute

type ServiceConfig struct {
	Enabled        bool
	WebhookURL     string
	WebhookSecret  string
	PollInterval   time.Duration
	RequestTimeout time.Duration
	RetryBase      time.Duration
	MaxAttempts    int
	BatchSize      int
}

type Service struct {
	config     ServiceConfig
	repository Repository
	client     *http.Client
	logger     *zap.Logger
	now        func() time.Time
}

func NewService(config ServiceConfig, repository Repository, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	client := &http.Client{Timeout: config.RequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Service{config: config, repository: repository, client: client, logger: logger, now: time.Now}
}

func (s *Service) Enabled() bool { return s.config.Enabled }

func (s *Service) Run(ctx context.Context) {
	if !s.config.Enabled {
		return
	}
	s.dispatch(ctx)
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dispatch(ctx)
		}
	}
}

func (s *Service) DispatchOnce(ctx context.Context) error {
	if !s.config.Enabled {
		return nil
	}
	staleBefore := s.now().UTC().Add(-(s.config.RequestTimeout + time.Minute))
	deliveries, err := s.repository.Claim(ctx, s.config.BatchSize, staleBefore)
	if err != nil {
		return err
	}
	for _, delivery := range deliveries {
		if err := s.deliver(ctx, delivery); err != nil {
			nextAttempt := s.now().UTC().Add(retryDelay(s.config.RetryBase, delivery.Attempts+1))
			if markErr := s.repository.MarkFailed(ctx, delivery.ID, s.config.MaxAttempts, nextAttempt, safeError(err, s.config.WebhookURL)); markErr != nil {
				return markErr
			}
			continue
		}
		if err := s.repository.MarkDelivered(ctx, delivery.ID, s.now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) List(ctx context.Context, filter ListFilter) (ListResponse, error) {
	return s.repository.List(ctx, filter)
}

// Enqueue writes a delivery into the outbox (used by the incident SLA monitor).
func (s *Service) Enqueue(ctx context.Context, input EnqueueInput) error {
	return s.repository.Enqueue(ctx, input)
}

func (s *Service) Retry(ctx context.Context, id int64) error {
	return s.repository.Retry(ctx, id)
}

func (s *Service) dispatch(ctx context.Context) {
	if err := s.DispatchOnce(ctx); err != nil && ctx.Err() == nil {
		s.logger.Error("dispatch diagnosis notifications", zap.Error(err))
	}
}

func (s *Service) deliver(ctx context.Context, delivery Delivery) error {
	body, err := json.Marshal(Envelope{ID: delivery.ID, EventType: delivery.EventType, OccurredAt: delivery.CreatedAt, Data: delivery.Payload})
	if err != nil {
		return fmt.Errorf("encode notification envelope: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "k8s-aiops-notifier/1.0")
	request.Header.Set("X-AIOps-Event-ID", fmt.Sprintf("%d", delivery.ID))
	request.Header.Set("X-AIOps-Event-Type", delivery.EventType)
	request.Header.Set("X-AIOps-Signature", sign(body, s.config.WebhookSecret))
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("send webhook request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func retryDelay(base time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for index := 1; index < attempt && delay < maximumRetryDelay; index++ {
		delay *= 2
		if delay > maximumRetryDelay {
			return maximumRetryDelay
		}
	}
	return delay
}

func safeError(err error, webhookURL string) string {
	message := strings.TrimSpace(strings.ReplaceAll(err.Error(), webhookURL, "<webhook>"))
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
