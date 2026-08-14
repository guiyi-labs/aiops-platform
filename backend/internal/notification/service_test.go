package notification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type repositoryStub struct {
	claimed       []Delivery
	claimCalls    int
	deliveredID   int64
	deliveredAt   time.Time
	enqueued      EnqueueInput
	failedID      int64
	failedMax     int
	failedNext    time.Time
	failedMessage string
	retryID       int64
	listResponse  ListResponse
}

func (r *repositoryStub) SetEnabled(context.Context, bool) error { return nil }
func (r *repositoryStub) Enqueue(_ context.Context, input EnqueueInput) error {
	r.enqueued = input
	return nil
}
func (r *repositoryStub) Claim(context.Context, int, time.Time) ([]Delivery, error) {
	r.claimCalls++
	return r.claimed, nil
}
func (r *repositoryStub) MarkDelivered(_ context.Context, id int64, at time.Time) error {
	r.deliveredID, r.deliveredAt = id, at
	return nil
}
func (r *repositoryStub) MarkFailed(_ context.Context, id int64, max int, next time.Time, message string) error {
	r.failedID, r.failedMax, r.failedNext, r.failedMessage = id, max, next, message
	return nil
}
func (r *repositoryStub) List(context.Context, ListFilter) (ListResponse, error) {
	return r.listResponse, nil
}
func (r *repositoryStub) Retry(_ context.Context, id int64) error { r.retryID = id; return nil }

func TestDispatchSignsAndDeliversWebhook(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	var receivedBody []byte
	var receivedSignature, receivedType string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		receivedBody, _ = io.ReadAll(request.Body)
		receivedSignature = request.Header.Get("X-AIOps-Signature")
		receivedType = request.Header.Get("X-AIOps-Event-Type")
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	createdAt := time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)
	repository := &repositoryStub{claimed: []Delivery{{ID: 42, EventType: "diagnosis.created", CreatedAt: createdAt, Payload: json.RawMessage(`{"diagnosis_id":7,"summary":"pod failed"}`)}}}
	service := NewService(ServiceConfig{Enabled: true, WebhookURL: server.URL, WebhookSecret: secret, RequestTimeout: time.Second, RetryBase: time.Second, MaxAttempts: 5, BatchSize: 10}, repository, nil)
	service.now = func() time.Time { return createdAt.Add(time.Minute) }

	if err := service.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if repository.deliveredID != 42 || repository.failedID != 0 {
		t.Fatalf("repository = %#v", repository)
	}
	if receivedSignature != sign(receivedBody, secret) || receivedType != "diagnosis.created" {
		t.Fatalf("signature=%q type=%q", receivedSignature, receivedType)
	}
	var envelope Envelope
	if err := json.Unmarshal(receivedBody, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.ID != 42 || envelope.EventType != "diagnosis.created" || !strings.Contains(string(envelope.Data), "pod failed") {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestDispatchRetriesWithoutPersistingResponseBodyOrSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte("provider-body-must-not-be-stored"))
	}))
	defer server.Close()

	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	repository := &repositoryStub{claimed: []Delivery{{ID: 9, EventType: "diagnosis.status_changed", Attempts: 1, CreatedAt: now, Payload: json.RawMessage(`{"status":"confirmed"}`)}}}
	service := NewService(ServiceConfig{Enabled: true, WebhookURL: server.URL, WebhookSecret: "secret-value-that-must-never-leak-123", RequestTimeout: time.Second, RetryBase: 10 * time.Second, MaxAttempts: 4, BatchSize: 5}, repository, nil)
	service.now = func() time.Time { return now }

	if err := service.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if repository.failedID != 9 || repository.failedMax != 4 || !repository.failedNext.Equal(now.Add(20*time.Second)) {
		t.Fatalf("repository = %#v", repository)
	}
	if repository.deliveredID != 0 || repository.failedMessage != "webhook returned HTTP 503" || strings.Contains(repository.failedMessage, "provider-body") || strings.Contains(repository.failedMessage, "secret-value") {
		t.Fatalf("failure message = %q", repository.failedMessage)
	}
}

func TestDisabledDispatcherDoesNotClaim(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(ServiceConfig{}, repository, nil)
	if err := service.DispatchOnce(context.Background()); err != nil || repository.claimCalls != 0 {
		t.Fatalf("err=%v claimCalls=%d", err, repository.claimCalls)
	}
}

func TestWebhookRedirectIsNotFollowed(t *testing.T) {
	redirected := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected++ }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Location", target.URL)
		response.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	repository := &repositoryStub{claimed: []Delivery{{ID: 5, EventType: "diagnosis.created", Payload: json.RawMessage(`{}`)}}}
	service := NewService(ServiceConfig{Enabled: true, WebhookURL: source.URL, WebhookSecret: "0123456789abcdef0123456789abcdef", RequestTimeout: time.Second, RetryBase: time.Second, MaxAttempts: 2, BatchSize: 1}, repository, nil)
	if err := service.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if redirected != 0 || repository.failedMessage != "webhook returned HTTP 307" {
		t.Fatalf("redirected=%d failure=%q", redirected, repository.failedMessage)
	}
}

func TestSafeErrorRedactsConfiguredWebhookURL(t *testing.T) {
	url := "https://example.com/hook?token=sensitive"
	if message := safeError(errors.New("POST "+url+" failed"), url); strings.Contains(message, "sensitive") {
		t.Fatalf("message = %q", message)
	}
}

func TestRetryDelayIsExponentiallyBounded(t *testing.T) {
	if retryDelay(10*time.Second, 3) != 40*time.Second || retryDelay(10*time.Minute, 3) != maximumRetryDelay {
		t.Fatal("unexpected retry delay")
	}
}

func TestServiceEnqueueForwardsToRepository(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(ServiceConfig{}, repository, nil)
	input := EnqueueInput{IncidentID: 9, EventType: EventTypeIncidentSLAEscalated, EscalationLevel: 2, Payload: `{"event":"incident.sla_escalated"}`}
	if err := service.Enqueue(context.Background(), input); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if repository.enqueued != input {
		t.Fatalf("enqueued = %#v, want %#v", repository.enqueued, input)
	}
}
