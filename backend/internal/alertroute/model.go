package alertroute

import (
	"errors"
	"time"
)

// Constants
const (
	MaxSilenceDuration     = 7 * 24 * time.Hour // hard maximum
	DefaultSilenceDuration = time.Hour
	MinSilenceDuration     = 5 * time.Minute
	MaxRoutesPerUser       = 50
	MaxReceiversPerUser    = 20
	MaxSilencesPerUser     = 30

	DeliveryStatusPending    = "pending"
	DeliveryStatusDelivering = "delivering"
	DeliveryStatusDelivered  = "delivered"
	DeliveryStatusDead       = "dead"

	MatchSeverityAny = "" // empty string means match any severity

	EventTypeFiring   = "firing"
	EventTypeResolved = "resolved"

	// M51 inhibit bounds (mirror silence bounds for consistency).
	MaxInhibitsPerUser = 30

	// Validation bounds
	MinReceiverNameLen = 1
	MaxReceiverNameLen = 64
	MinSecretLen       = 32
	MinPriority        = 1
	MaxPriority        = 100
	MinReasonLen       = 1
	MaxReasonLen       = 500

	MinGroupInterval  = 30 * time.Second
	MaxGroupInterval  = time.Hour
	MinRepeatInterval = time.Minute
	MaxRepeatInterval = 24 * time.Hour

	// Delivery defaults
	DefaultMaxAttempts    = 5
	DefaultRetryBase      = 10 * time.Second
	DefaultBatchSize      = 10
	DefaultRequestTimeout = 10 * time.Second
	MaxRetryDelay         = 15 * time.Minute
	MaxResponseRead       = 4096
	MaxLastErrorLen       = 500

	SignatureHeader = "X-AIOps-Signature"
)

var (
	ErrReceiverNotFound      = errors.New("alert receiver not found")
	ErrReceiverInUse         = errors.New("receiver is referenced by routes")
	ErrRouteNotFound         = errors.New("alert route not found")
	ErrSilenceNotFound       = errors.New("alert silence not found")
	ErrInhibitNotFound       = errors.New("alert inhibit not found")
	ErrDeliveryNotFound      = errors.New("alert route delivery not found")
	ErrInvalidReceiver       = errors.New("alert receiver validation failed")
	ErrInvalidRoute          = errors.New("alert route validation failed")
	ErrInvalidSilence        = errors.New("alert silence validation failed")
	ErrInvalidInhibit        = errors.New("alert inhibit validation failed")
	ErrPermanentSilence      = errors.New("permanent silence is forbidden")
	ErrSilenceExpired        = errors.New("silence has expired")
	ErrReceiverLimit         = errors.New("receiver limit reached for user")
	ErrRouteLimit            = errors.New("route limit reached for user")
	ErrSilenceLimit          = errors.New("silence limit reached for user")
	ErrInhibitLimit          = errors.New("inhibit limit reached for user")
	ErrDuplicateReceiverName = errors.New("receiver name already exists for user")
)

// Receiver — a webhook destination
type Receiver struct {
	ID        int64
	Name      string // 1..64 chars, unique per user
	URL       string // HTTPS URL, stored encrypted in DB
	Secret    string // HMAC signing secret, stored encrypted, >=32 chars in production
	CreatorID int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Route — exact-match rule mapping alert to receiver
type Route struct {
	ID             int64
	ReceiverID     int64
	CreatorID      int64
	Priority       int            // 1..100, lower = higher priority
	ClusterID      *int64         // nil = match any cluster
	RuleName       string         // empty = match any rule (display name from alert.Rule)
	Severity       string         // empty = match any severity
	DedupeKey      string         // template: "{{.ClusterID}}:{{.RuleName}}" etc.
	GroupInterval  *time.Duration // optional, min 30s, max 1h
	RepeatInterval *time.Duration // optional, min 1m, max 24h
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Silence — time-bounded suppression
type Silence struct {
	ID        int64
	CreatorID int64
	ClusterID *int64 // nil = match any cluster
	RuleName  string // empty = match any rule
	Severity  string // empty = match any severity
	Reason    string // mandatory, 1..500 chars
	StartsAt  time.Time
	EndsAt    time.Time // must be in future, max MaxSilenceDuration from StartsAt
	CreatedAt time.Time
}

// Inhibit — source_match -> target_match suppression (M51). Unlike a silence,
// an inhibit is not time-bounded: while any firing alert matches the source,
// alerts matching the target are suppressed. The source is considered firing
// when a non-resolved delivery exists for the source match within the active
// window; the dispatch loop re-evaluates IsInhibited on every MatchAndDeliver
// call. Reason is mandatory for auditability (mirrors silences).
type Inhibit struct {
	ID              int64
	CreatorID       int64
	SourceClusterID *int64 // nil = match any cluster
	SourceRuleName  string // empty = match any rule
	SourceSeverity  string // empty = match any severity
	TargetClusterID *int64 // nil = match any cluster
	TargetRuleName  string // empty = match any rule
	TargetSeverity  string // empty = match any severity
	Reason          string // mandatory, 1..500 chars
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Delivery — delivery record for audit and retry
type Delivery struct {
	ID              int64
	RouteID         int64
	ReceiverID      int64
	AlertInstanceID int64
	ClusterID       int64
	RuleName        string
	Severity        string
	EventType       string // "firing", "resolved"
	DedupeKey       string
	Status          string
	Attempts        int
	NextAttemptAt   time.Time
	DeliveredAt     *time.Time
	LastError       string
	LockedAt        *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// DeliveryView — public projection for delivery records. It carries no
// secrets (deliveries store none) and uses JSON tags consistent with the
// other view types.
type DeliveryView struct {
	ID              int64      `json:"id"`
	RouteID         int64      `json:"route_id"`
	ReceiverID      int64      `json:"receiver_id"`
	AlertInstanceID int64      `json:"alert_instance_id"`
	EventType       string     `json:"event_type"`
	DedupeKey       string     `json:"dedupe_key"`
	Status          string     `json:"status"`
	Attempts        int        `json:"attempts"`
	NextAttemptAt   time.Time  `json:"next_attempt_at"`
	DeliveredAt     *time.Time `json:"delivered_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ReceiverView — public projection (no URL/Secret)
type ReceiverView struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URLMasked string `json:"url_masked"` // "https://***.example.com/webhook"
	CreatorID int64  `json:"creator_id"`
}

// RouteView — public projection
type RouteView struct {
	ID             int64          `json:"id"`
	ReceiverID     int64          `json:"receiver_id"`
	ReceiverName   string         `json:"receiver_name"`
	Priority       int            `json:"priority"`
	ClusterID      *int64         `json:"cluster_id,omitempty"`
	RuleName       string         `json:"rule_name"`
	Severity       string         `json:"severity"`
	DedupeKey      string         `json:"dedupe_key"`
	GroupInterval  *time.Duration `json:"group_interval,omitempty"`
	RepeatInterval *time.Duration `json:"repeat_interval,omitempty"`
	Enabled        bool           `json:"enabled"`
}

// SilenceView — public projection
type SilenceView struct {
	ID        int64     `json:"id"`
	ClusterID *int64    `json:"cluster_id,omitempty"`
	RuleName  string    `json:"rule_name"`
	Severity  string    `json:"severity"`
	Reason    string    `json:"reason"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	CreatorID int64     `json:"creator_id"`
}

// InhibitView — public projection (M51).
type InhibitView struct {
	ID              int64  `json:"id"`
	SourceClusterID *int64 `json:"source_cluster_id,omitempty"`
	SourceRuleName  string `json:"source_rule_name"`
	SourceSeverity  string `json:"source_severity"`
	TargetClusterID *int64 `json:"target_cluster_id,omitempty"`
	TargetRuleName  string `json:"target_rule_name"`
	TargetSeverity  string `json:"target_severity"`
	Reason          string `json:"reason"`
	Enabled         bool   `json:"enabled"`
	CreatorID       int64  `json:"creator_id"`
}

// MatchAlert — input to route matching
type MatchAlert struct {
	AlertInstanceID int64
	ClusterID       int64
	RuleName        string
	Severity        string
	EventType       string // "firing" or "resolved"
}

// WebhookEnvelope — payload delivered to webhook receivers
type WebhookEnvelope struct {
	ID              int64     `json:"id"`
	EventType       string    `json:"event_type"`
	DedupeKey       string    `json:"dedupe_key"`
	ClusterID       int64     `json:"cluster_id"`
	RuleName        string    `json:"rule_name"`
	Severity        string    `json:"severity"`
	AlertInstanceID int64     `json:"alert_instance_id"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// TableName methods
func (Receiver) TableName() string { return "alert_route_receivers" }
func (Route) TableName() string    { return "alert_routes" }
func (Silence) TableName() string  { return "alert_silences" }
func (Inhibit) TableName() string  { return "alert_inhibits" }
func (Delivery) TableName() string { return "alert_route_deliveries" }
