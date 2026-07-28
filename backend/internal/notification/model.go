package notification

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrDeliveryNotFound = errors.New("notification delivery not found")
var ErrDeliveryNotRetryable = errors.New("notification delivery is not retryable")

type Delivery struct {
	ID            int64           `json:"id"`
	DiagnosisID   int64           `json:"diagnosis_id"`
	EventType     string          `json:"event_type"`
	Status        string          `json:"status"`
	Attempts      int             `json:"attempts"`
	NextAttemptAt time.Time       `json:"next_attempt_at"`
	DeliveredAt   *time.Time      `json:"delivered_at,omitempty"`
	LastError     string          `json:"last_error,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Payload       json.RawMessage `json:"-"`
}

type ListFilter struct {
	DiagnosisID int64
	EventType   string
	Status      string
	Limit       int
}

type ListResponse struct {
	Items     []Delivery `json:"items"`
	Total     int64      `json:"total"`
	Remaining int64      `json:"remaining"`
}

type Envelope struct {
	ID         int64           `json:"id"`
	EventType  string          `json:"event_type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Data       json.RawMessage `json:"data"`
}
