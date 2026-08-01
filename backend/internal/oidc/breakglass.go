package oidc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// BreakGlassEvent records a single break-glass login. Break-glass accounts are
// one or two local accounts retained so operator access survives a provider
// outage; their use must produce high-priority audit records and be exercised
// on a bounded drill interval (ADR 0052).
type BreakGlassEvent struct {
	UserID     int64
	Username   string
	UserAgent  string
	IPAddress  string
	Reason     string
	OccurredAt time.Time
}

// BreakGlassAuditor records break-glass login events for high-priority audit
// and operational drill tracking. The implementation is expected to forward
// the event to the platform audit trail at high priority and to notify
// operations staff.
type BreakGlassAuditor interface {
	RecordBreakGlassLogin(ctx context.Context, event BreakGlassEvent) error
}

// BreakGlassDrillConfig bounds the break-glass drill interval. A drill is a
// controlled exercise of a break-glass account to prove the fallback path
// works; the operational policy requires drills on a bounded interval so the
// credentials remain fresh and the audit path is exercised.
type BreakGlassDrillConfig struct {
	// RequiredInterval is the maximum allowed gap between break-glass drills.
	// If no drill has been recorded within this window, IsCurrent returns false
	// so the readiness gate can flag the fallback as stale.
	RequiredInterval time.Duration
	// MaxAccounts is the maximum number of break-glass accounts permitted.
	// ADR 0052 allows one or two.
	MaxAccounts int
}

// BreakGlassDrillTracker records break-glass login events and reports whether
// the drill interval is current. It is safe for concurrent use. The tracker is
// an in-memory readiness probe; the authoritative audit trail is written by
// the BreakGlassAuditor, which the HTTP handler calls alongside Record.
type BreakGlassDrillTracker struct {
	cfg     BreakGlassDrillConfig
	auditor BreakGlassAuditor
	now     func() time.Time

	mu   sync.Mutex
	last time.Time
}

// NewBreakGlassDrillTracker constructs a tracker bound to the auditor. The
// auditor may be nil when drill tracking is used without forwarding to the
// audit trail (for example in tests); production wiring supplies a real
// auditor.
func NewBreakGlassDrillTracker(cfg BreakGlassDrillConfig, auditor BreakGlassAuditor) *BreakGlassDrillTracker {
	if cfg.RequiredInterval <= 0 {
		cfg.RequiredInterval = 7 * 24 * time.Hour
	}
	if cfg.MaxAccounts <= 0 {
		cfg.MaxAccounts = 2
	}
	if cfg.MaxAccounts > 2 {
		cfg.MaxAccounts = 2
	}
	return &BreakGlassDrillTracker{
		cfg:     cfg,
		auditor: auditor,
		now:     time.Now,
	}
}

// RequiredInterval returns the maximum allowed gap between break-glass drills.
func (t *BreakGlassDrillTracker) RequiredInterval() time.Duration { return t.cfg.RequiredInterval }

// MaxAccounts returns the maximum number of break-glass accounts permitted.
func (t *BreakGlassDrillTracker) MaxAccounts() int { return t.cfg.MaxAccounts }

// Record logs a break-glass login event. It forwards the event to the auditor
// (when configured) and updates the last-drill timestamp so IsCurrent reports
// the fallback as exercised. The reason must be non-empty so every break-glass
// use is attributed to a drill or an operational outage.
func (t *BreakGlassDrillTracker) Record(ctx context.Context, event BreakGlassEvent) error {
	if event.UserID == 0 || event.Username == "" {
		return fmt.Errorf("oidc: break-glass event requires user id and username")
	}
	if event.Reason == "" {
		return fmt.Errorf("oidc: break-glass event requires a reason (drill or outage)")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = t.now()
	}
	if t.auditor != nil {
		if err := t.auditor.RecordBreakGlassLogin(ctx, event); err != nil {
			return fmt.Errorf("oidc: record break-glass audit: %w", err)
		}
	}
	t.mu.Lock()
	t.last = event.OccurredAt
	t.mu.Unlock()
	return nil
}

// IsCurrent reports whether a break-glass drill has been recorded within the
// required interval. The readiness gate uses this to flag a stale fallback.
func (t *BreakGlassDrillTracker) IsCurrent() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last.IsZero() {
		return false
	}
	return t.now().Sub(t.last) <= t.cfg.RequiredInterval
}

// LastDrillAt returns the timestamp of the most recent break-glass drill, or
// the zero time when none has been recorded.
func (t *BreakGlassDrillTracker) LastDrillAt() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.last
}

// ErrBreakGlassStale indicates the break-glass fallback has not been exercised
// within the required drill interval. The readiness gate surfaces this so
// operations staff can schedule a drill before the fallback is needed.
var ErrBreakGlassStale = errors.New("oidc: break-glass drill is stale")
