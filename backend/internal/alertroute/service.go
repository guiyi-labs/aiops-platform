package alertroute

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"go.uber.org/zap"
)

// Service implements alert route matching, silence suppression and webhook delivery.
type Service struct {
	repo           Repository
	now            func() time.Time
	client         *http.Client
	logger         *zap.Logger
	maxAttempts    int
	retryBase      time.Duration
	batchSize      int
	requestTimeout time.Duration
	cipher         SecretCipher
}

// SecretCipher is implemented by cluster.Encryptor. Receiver URLs and HMAC
// secrets use the same versioned AEAD envelope as cluster credentials.
type SecretCipher interface {
	Encrypt([]byte) ([]byte, string, error)
	Decrypt([]byte, string) ([]byte, error)
}

// NewService wires the service with sensible delivery defaults.
func NewService(repo Repository, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	timeout := DefaultRequestTimeout
	return &Service{
		repo:           repo,
		now:            time.Now,
		client:         &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		logger:         logger,
		maxAttempts:    DefaultMaxAttempts,
		retryBase:      DefaultRetryBase,
		batchSize:      DefaultBatchSize,
		requestTimeout: timeout,
	}
}

// WithCipher enables encrypted-at-rest receiver credentials. Production
// wiring must call this; nil is retained only for isolated unit tests.
func (s *Service) WithCipher(cipher SecretCipher) *Service {
	s.cipher = cipher
	return s
}

// ConfigureDelivery applies validated runtime delivery settings.
func (s *Service) ConfigureDelivery(requestTimeout, retryBase time.Duration, maxAttempts, batchSize int) {
	if requestTimeout > 0 {
		s.requestTimeout = requestTimeout
		s.client.Timeout = requestTimeout
	}
	if retryBase > 0 {
		s.retryBase = retryBase
	}
	if maxAttempts > 0 {
		s.maxAttempts = maxAttempts
	}
	if batchSize > 0 {
		s.batchSize = batchSize
	}
}

// --- Receiver CRUD ---

// CreateReceiver validates and persists a new webhook receiver.
func (s *Service) CreateReceiver(ctx context.Context, creatorID int64, name, rawURL, secret string) (Receiver, error) {
	if err := validateReceiver(name, rawURL, secret); err != nil {
		return Receiver{}, err
	}
	existing, err := s.repo.ListReceivers(ctx, creatorID)
	if err != nil {
		return Receiver{}, err
	}
	if len(existing) >= MaxReceiversPerUser {
		return Receiver{}, ErrReceiverLimit
	}
	for _, r := range existing {
		if strings.EqualFold(r.Name, name) {
			return Receiver{}, ErrDuplicateReceiverName
		}
	}
	now := s.now().UTC()
	storedURL, err := s.encryptValue(rawURL)
	if err != nil {
		return Receiver{}, err
	}
	storedSecret, err := s.encryptValue(secret)
	if err != nil {
		return Receiver{}, err
	}
	receiver := Receiver{Name: name, URL: storedURL, Secret: storedSecret, CreatorID: creatorID, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateReceiver(ctx, &receiver); err != nil {
		return Receiver{}, err
	}
	receiver.URL = rawURL
	receiver.Secret = ""
	return receiver, nil
}

func (s *Service) GetReceiver(ctx context.Context, id, creatorID int64) (Receiver, error) {
	return s.repo.GetReceiver(ctx, id, creatorID)
}

func (s *Service) ListReceivers(ctx context.Context, creatorID int64) ([]ReceiverView, error) {
	receivers, err := s.repo.ListReceivers(ctx, creatorID)
	if err != nil {
		return nil, err
	}
	views := make([]ReceiverView, 0, len(receivers))
	for _, r := range receivers {
		rawURL, decryptErr := s.decryptValue(r.URL)
		if decryptErr != nil {
			return nil, decryptErr
		}
		views = append(views, ReceiverView{ID: r.ID, Name: r.Name, URLMasked: maskURL(rawURL), CreatorID: r.CreatorID})
	}
	return views, nil
}

func (s *Service) DeleteReceiver(ctx context.Context, id, creatorID int64) error {
	if _, err := s.repo.GetReceiver(ctx, id, creatorID); err != nil {
		return err
	}
	return s.repo.DeleteReceiver(ctx, id, creatorID)
}

// --- Route CRUD ---

func (s *Service) CreateRoute(ctx context.Context, route *Route) (Route, error) {
	if err := validateRoute(route); err != nil {
		return Route{}, err
	}
	if _, err := s.repo.GetReceiver(ctx, route.ReceiverID, route.CreatorID); err != nil {
		return Route{}, err
	}
	existing, err := s.repo.ListRoutes(ctx, route.CreatorID)
	if err != nil {
		return Route{}, err
	}
	if len(existing) >= MaxRoutesPerUser {
		return Route{}, ErrRouteLimit
	}
	now := s.now().UTC()
	route.CreatedAt = now
	route.UpdatedAt = now
	route.Enabled = true
	if err := s.repo.CreateRoute(ctx, route); err != nil {
		return Route{}, err
	}
	return *route, nil
}

func (s *Service) GetRoute(ctx context.Context, id, creatorID int64) (Route, error) {
	return s.repo.GetRoute(ctx, id, creatorID)
}

func (s *Service) ListRoutes(ctx context.Context, creatorID int64) ([]RouteView, error) {
	routes, err := s.repo.ListRoutes(ctx, creatorID)
	if err != nil {
		return nil, err
	}
	receivers, err := s.repo.ListReceivers(ctx, creatorID)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]string, len(receivers))
	for _, r := range receivers {
		byID[r.ID] = r.Name
	}
	views := make([]RouteView, 0, len(routes))
	for _, route := range routes {
		views = append(views, RouteView{
			ID: route.ID, ReceiverID: route.ReceiverID, ReceiverName: byID[route.ReceiverID],
			Priority: route.Priority, ClusterID: route.ClusterID, RuleName: route.RuleName,
			Severity: route.Severity, DedupeKey: route.DedupeKey,
			GroupInterval: route.GroupInterval, RepeatInterval: route.RepeatInterval, Enabled: route.Enabled,
		})
	}
	return views, nil
}

func (s *Service) UpdateRoute(ctx context.Context, id, creatorID int64, input PatchRouteInput) (Route, error) {
	if input.Priority != nil && (*input.Priority < MinPriority || *input.Priority > MaxPriority) {
		return Route{}, ErrInvalidRoute
	}
	if input.GroupInterval != nil && (*input.GroupInterval < MinGroupInterval || *input.GroupInterval > MaxGroupInterval) {
		return Route{}, ErrInvalidRoute
	}
	if input.RepeatInterval != nil && (*input.RepeatInterval < MinRepeatInterval || *input.RepeatInterval > MaxRepeatInterval) {
		return Route{}, ErrInvalidRoute
	}
	if _, err := s.repo.GetRoute(ctx, id, creatorID); err != nil {
		return Route{}, err
	}
	return s.repo.UpdateRoute(ctx, id, creatorID, input)
}

func (s *Service) DeleteRoute(ctx context.Context, id, creatorID int64) error {
	if _, err := s.repo.GetRoute(ctx, id, creatorID); err != nil {
		return err
	}
	return s.repo.DeleteRoute(ctx, id, creatorID)
}

// --- Silence CRUD ---

func (s *Service) CreateSilence(ctx context.Context, silence *Silence) (Silence, error) {
	if err := validateSilence(silence, s.now()); err != nil {
		return Silence{}, err
	}
	existing, err := s.repo.ListSilences(ctx, SilenceListFilter{CreatorID: &silence.CreatorID})
	if err != nil {
		return Silence{}, err
	}
	if len(existing) >= MaxSilencesPerUser {
		return Silence{}, ErrSilenceLimit
	}
	silence.CreatedAt = s.now().UTC()
	if err := s.repo.CreateSilence(ctx, silence); err != nil {
		return Silence{}, err
	}
	return *silence, nil
}

func (s *Service) ListSilences(ctx context.Context, filter SilenceListFilter) ([]SilenceView, error) {
	silences, err := s.repo.ListSilences(ctx, filter)
	if err != nil {
		return nil, err
	}
	views := make([]SilenceView, 0, len(silences))
	for _, s2 := range silences {
		views = append(views, SilenceView{
			ID: s2.ID, ClusterID: s2.ClusterID, RuleName: s2.RuleName, Severity: s2.Severity,
			Reason: s2.Reason, StartsAt: s2.StartsAt, EndsAt: s2.EndsAt, CreatorID: s2.CreatorID,
		})
	}
	return views, nil
}

func (s *Service) DeleteSilence(ctx context.Context, id, creatorID int64) error {
	return s.repo.DeleteSilence(ctx, id, creatorID)
}

// --- Inhibits (M51) ---

// DefaultInhibitActiveWindow bounds how long a firing source delivery is
// considered "active" for inhibition purposes. Mirrors the silence evaluation
// cadence; a source alert whose last delivery is older than this window is
// treated as no longer firing (the alert is assumed resolved/stale).
const DefaultInhibitActiveWindow = 5 * time.Minute

// CreateInhibit validates and persists a new inhibit rule.
func (s *Service) CreateInhibit(ctx context.Context, inhibit *Inhibit) (Inhibit, error) {
	if err := validateInhibit(inhibit); err != nil {
		return Inhibit{}, err
	}
	existing, err := s.repo.ListInhibits(ctx, InhibitListFilter{CreatorID: &inhibit.CreatorID})
	if err != nil {
		return Inhibit{}, err
	}
	if len(existing) >= MaxInhibitsPerUser {
		return Inhibit{}, ErrInhibitLimit
	}
	now := s.now().UTC()
	inhibit.CreatedAt = now
	inhibit.UpdatedAt = now
	// Inhibits are active on creation (mirrors the migration DEFAULT TRUE).
	// The Enabled flag supports later disabling via a future update path.
	inhibit.Enabled = true
	if err := s.repo.CreateInhibit(ctx, inhibit); err != nil {
		return Inhibit{}, err
	}
	return *inhibit, nil
}

// ListInhibits returns inhibits matching the filter as public views.
func (s *Service) ListInhibits(ctx context.Context, filter InhibitListFilter) ([]InhibitView, error) {
	inhibits, err := s.repo.ListInhibits(ctx, filter)
	if err != nil {
		return nil, err
	}
	views := make([]InhibitView, 0, len(inhibits))
	for _, inh := range inhibits {
		views = append(views, InhibitView{
			ID: inh.ID, SourceClusterID: inh.SourceClusterID, SourceRuleName: inh.SourceRuleName,
			SourceSeverity: inh.SourceSeverity, TargetClusterID: inh.TargetClusterID,
			TargetRuleName: inh.TargetRuleName, TargetSeverity: inh.TargetSeverity,
			Reason: inh.Reason, Enabled: inh.Enabled, CreatorID: inh.CreatorID,
		})
	}
	return views, nil
}

// DeleteInhibit removes an inhibit rule. Only the creator may delete it.
func (s *Service) DeleteInhibit(ctx context.Context, id, creatorID int64) error {
	return s.repo.DeleteInhibit(ctx, id, creatorID)
}

// IsInhibited reports whether an enabled inhibit rule suppresses the target
// alert. The target is inhibited when (a) an enabled inhibit's target match
// matches the alert AND (b) the inhibit's source match has at least one firing
// (non-resolved) delivery within the active window.
func (s *Service) IsInhibited(ctx context.Context, alert MatchAlert) (bool, *Inhibit) {
	inhibits, err := s.repo.ListEnabledInhibits(ctx)
	if err != nil {
		return false, nil
	}
	for i := range inhibits {
		inh := &inhibits[i]
		if !inhibitTargetMatches(inh, alert) {
			continue
		}
		source := MatchAlert{
			ClusterID: alert.ClusterID,
			RuleName:  inh.SourceRuleName,
			Severity:  inh.SourceSeverity,
		}
		if inh.SourceClusterID != nil {
			source.ClusterID = *inh.SourceClusterID
		}
		firing, err := s.repo.HasFiringSource(ctx, source, DefaultInhibitActiveWindow)
		if err != nil {
			return false, nil
		}
		if firing {
			return true, inh
		}
	}
	return false, nil
}

// ListDeliveries returns a paginated list of delivery records as public views.
// It is a thin projection over the repository so the HTTP layer never handles
// the persistence-shaped Delivery struct directly.
func (s *Service) ListDeliveries(ctx context.Context, filter DeliveryListFilter) (ListResponse[DeliveryView], error) {
	response, err := s.repo.ListDeliveries(ctx, filter)
	if err != nil {
		return ListResponse[DeliveryView]{}, err
	}
	views := make([]DeliveryView, 0, len(response.Items))
	for _, d := range response.Items {
		views = append(views, DeliveryView{
			ID: d.ID, RouteID: d.RouteID, ReceiverID: d.ReceiverID,
			AlertInstanceID: d.AlertInstanceID, EventType: d.EventType,
			DedupeKey: d.DedupeKey, Status: d.Status, Attempts: d.Attempts,
			NextAttemptAt: d.NextAttemptAt, DeliveredAt: d.DeliveredAt,
			LastError: d.LastError, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		})
	}
	return ListResponse[DeliveryView]{Items: views, Total: response.Total}, nil
}

// --- Matching and delivery ---

// IsSilenced reports whether an active silence matches the alert.
func (s *Service) IsSilenced(ctx context.Context, alert MatchAlert) (bool, *Silence) {
	silences, err := s.repo.ListActiveSilences(ctx, s.now())
	if err != nil {
		return false, nil
	}
	for i := range silences {
		if silenceMatches(silences[i], alert) {
			return true, &silences[i]
		}
	}
	return false, nil
}

// MatchAndDeliver finds enabled routes matching the alert and creates delivery records.
// It honours active silences, inhibit rules (M51) and the dedupe contract —
// duplicate invocations produce a single business delivery per
// (route, dedupe key, event type).
func (s *Service) MatchAndDeliver(ctx context.Context, alert MatchAlert) error {
	if silenced, _ := s.IsSilenced(ctx, alert); silenced {
		return nil
	}
	if inhibited, _ := s.IsInhibited(ctx, alert); inhibited {
		return nil
	}
	routes, err := s.repo.ListEnabledRoutes(ctx)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	for _, route := range routes {
		if !routeMatches(route, alert) {
			continue
		}
		dedupeKey, err := renderDedupeKey(route.DedupeKey, alert)
		if err != nil || dedupeKey == "" {
			return fmt.Errorf("render dedupe key for route %d: %w", route.ID, err)
		}
		existing, err := s.repo.FindActiveDelivery(ctx, route.ID, dedupeKey, alert.EventType)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Status == DeliveryStatusPending || existing.Status == DeliveryStatusDelivering {
				continue
			}
			// A resolved notification is terminal for this route/dedupe key.
			// Firing notifications repeat only when the operator configured a
			// repeat interval and that interval has elapsed.
			if alert.EventType != EventTypeFiring || route.RepeatInterval == nil || existing.DeliveredAt == nil || now.Before(existing.DeliveredAt.Add(*route.RepeatInterval)) {
				continue
			}
		}
		nextAttempt := now
		if route.GroupInterval != nil {
			nextAttempt = nextAttempt.Add(*route.GroupInterval)
		}
		delivery := Delivery{
			RouteID:         route.ID,
			ReceiverID:      route.ReceiverID,
			AlertInstanceID: alert.AlertInstanceID,
			ClusterID:       alert.ClusterID,
			RuleName:        alert.RuleName,
			Severity:        alert.Severity,
			EventType:       alert.EventType,
			DedupeKey:       dedupeKey,
			Status:          DeliveryStatusPending,
			Attempts:        0,
			NextAttemptAt:   nextAttempt,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := s.repo.CreateDelivery(ctx, &delivery); err != nil {
			return err
		}
	}
	return nil
}

// DispatchOnce claims a batch of due deliveries and pushes them to webhook receivers.
func (s *Service) DispatchOnce(ctx context.Context) error {
	now := s.now().UTC()
	deliveries, err := s.repo.ClaimDeliveries(ctx, s.batchSize, now)
	if err != nil {
		return err
	}
	for _, delivery := range deliveries {
		if err := s.deliver(ctx, delivery, now); err != nil {
			nextAttempt := now.Add(retryDelay(s.retryBase, delivery.Attempts+1))
			if markErr := s.repo.MarkFailed(ctx, delivery.ID, s.maxAttempts, nextAttempt, sanitizeError(err)); markErr != nil {
				return markErr
			}
			continue
		}
		if err := s.repo.MarkDelivered(ctx, delivery.ID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) deliver(ctx context.Context, delivery Delivery, now time.Time) error {
	receiver, err := s.repo.GetReceiverByID(ctx, delivery.ReceiverID)
	if err != nil {
		return err
	}
	envelope := WebhookEnvelope{
		ID: delivery.ID, EventType: delivery.EventType, DedupeKey: delivery.DedupeKey,
		ClusterID: delivery.ClusterID, RuleName: delivery.RuleName, Severity: delivery.Severity,
		AlertInstanceID: delivery.AlertInstanceID, OccurredAt: now,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encode webhook envelope: %w", err)
	}
	rawURL, err := s.decryptValue(receiver.URL)
	if err != nil {
		return fmt.Errorf("decrypt receiver URL: %w", err)
	}
	secret, err := s.decryptValue(receiver.Secret)
	if err != nil {
		return fmt.Errorf("decrypt receiver secret: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "k8s-aiops-alertroute/1.0")
	request.Header.Set("X-AIOps-Event-ID", fmt.Sprintf("%d", delivery.ID))
	request.Header.Set("X-AIOps-Event-Type", delivery.EventType)
	request.Header.Set(SignatureHeader, sign(body, secret))
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("send webhook request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, MaxResponseRead))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}

// --- Validation ---

func validateReceiver(name, rawURL, secret string) error {
	if len(name) < MinReceiverNameLen || len(name) > MaxReceiverNameLen {
		return ErrInvalidReceiver
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ErrInvalidReceiver
	}
	if host := parsed.Hostname(); host == "localhost" || isPrivateAddress(net.ParseIP(host)) {
		return ErrInvalidReceiver
	}
	if len(secret) < MinSecretLen {
		return ErrInvalidReceiver
	}
	return nil
}

func validateRoute(route *Route) error {
	if route == nil {
		return ErrInvalidRoute
	}
	if route.Priority < MinPriority || route.Priority > MaxPriority {
		return ErrInvalidRoute
	}
	if strings.TrimSpace(route.DedupeKey) == "" {
		return ErrInvalidRoute
	}
	if route.GroupInterval != nil && (*route.GroupInterval < MinGroupInterval || *route.GroupInterval > MaxGroupInterval) {
		return ErrInvalidRoute
	}
	if route.RepeatInterval != nil && (*route.RepeatInterval < MinRepeatInterval || *route.RepeatInterval > MaxRepeatInterval) {
		return ErrInvalidRoute
	}
	return nil
}

func validateSilence(silence *Silence, now time.Time) error {
	if silence == nil {
		return ErrInvalidSilence
	}
	if len(silence.Reason) < MinReasonLen || len(silence.Reason) > MaxReasonLen {
		return ErrInvalidSilence
	}
	if !silence.StartsAt.Before(silence.EndsAt) {
		return ErrInvalidSilence
	}
	if silence.EndsAt.Sub(silence.StartsAt) > MaxSilenceDuration {
		return ErrPermanentSilence
	}
	if !silence.EndsAt.After(now) {
		return ErrSilenceExpired
	}
	return nil
}

// --- Matching helpers ---

func routeMatches(route Route, alert MatchAlert) bool {
	if route.ClusterID != nil && *route.ClusterID != alert.ClusterID {
		return false
	}
	if route.RuleName != "" && route.RuleName != alert.RuleName {
		return false
	}
	if route.Severity != "" && route.Severity != alert.Severity {
		return false
	}
	return true
}

func silenceMatches(silence Silence, alert MatchAlert) bool {
	if silence.ClusterID != nil && *silence.ClusterID != alert.ClusterID {
		return false
	}
	if silence.RuleName != "" && silence.RuleName != alert.RuleName {
		return false
	}
	if silence.Severity != "" && silence.Severity != alert.Severity {
		return false
	}
	return true
}

// inhibitTargetMatches reports whether the alert matches the inhibit's target
// matchers (cluster/rule/severity). Source matching is handled separately via
// HasFiringSource because the source is a delivery-state query, not a static
// match against the incoming alert.
func inhibitTargetMatches(inh *Inhibit, alert MatchAlert) bool {
	if inh.TargetClusterID != nil && *inh.TargetClusterID != alert.ClusterID {
		return false
	}
	if inh.TargetRuleName != "" && inh.TargetRuleName != alert.RuleName {
		return false
	}
	if inh.TargetSeverity != "" && inh.TargetSeverity != alert.Severity {
		return false
	}
	return true
}

// validateInhibit enforces the M51 inhibit contract: reason is mandatory
// (1..500 chars, mirroring silences) and at least one source matcher and one
// target matcher must be non-empty (a fully-wildcard inhibit on both sides is
// rejected because it would suppress all alerts whenever any alert fires).
func validateInhibit(inh *Inhibit) error {
	if inh == nil {
		return ErrInvalidInhibit
	}
	if len(inh.Reason) < MinReasonLen || len(inh.Reason) > MaxReasonLen {
		return ErrInvalidInhibit
	}
	sourceEmpty := inh.SourceClusterID == nil && inh.SourceRuleName == "" && inh.SourceSeverity == ""
	targetEmpty := inh.TargetClusterID == nil && inh.TargetRuleName == "" && inh.TargetSeverity == ""
	if sourceEmpty || targetEmpty {
		return ErrInvalidInhibit
	}
	return nil
}

func renderDedupeKey(templateText string, alert MatchAlert) (string, error) {
	if !strings.Contains(templateText, "{{") {
		return templateText, nil
	}
	tmpl, err := template.New("dedupe").Parse(templateText)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"ClusterID":       alert.ClusterID,
		"RuleName":        alert.RuleName,
		"Severity":        alert.Severity,
		"EventType":       alert.EventType,
		"AlertInstanceID": alert.AlertInstanceID,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func maskURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "***"
	}
	return parsed.Scheme + "://" + parsed.Host + "/***"
}

func isPrivateAddress(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast())
}

// MaskURL returns a masked representation of a webhook URL for API responses.
// It is the exported wrapper over the internal maskURL so the HTTP layer can
// build ReceiverView responses without duplicating the masking logic.
func MaskURL(rawURL string) string { return maskURL(rawURL) }

func (s *Service) encryptValue(value string) (string, error) {
	if s.cipher == nil {
		return value, nil
	}
	ciphertext, version, err := s.cipher.Encrypt([]byte(value))
	if err != nil {
		return "", err
	}
	return "enc:" + version + ":" + base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

func (s *Service) decryptValue(value string) (string, error) {
	if !strings.HasPrefix(value, "enc:") {
		if s.cipher == nil {
			return value, nil
		}
		return "", fmt.Errorf("receiver credential is not encrypted")
	}
	parts := strings.SplitN(value, ":", 3)
	if len(parts) != 3 || s.cipher == nil {
		return "", fmt.Errorf("receiver credential envelope is invalid")
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode receiver credential: %w", err)
	}
	plaintext, err := s.cipher.Decrypt(ciphertext, parts[1])
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
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
	for index := 1; index < attempt && delay < MaxRetryDelay; index++ {
		delay *= 2
		if delay > MaxRetryDelay {
			return MaxRetryDelay
		}
	}
	return delay
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > MaxLastErrorLen {
		message = message[:MaxLastErrorLen]
	}
	return message
}
