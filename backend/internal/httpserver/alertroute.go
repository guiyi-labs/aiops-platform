package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/alertroute"
)

// alertrouteHandler exposes the M37B alert-route service: webhook receivers,
// exact-match routes, time-bounded silences and delivery records. All write
// operations are scoped to the authenticated user (currentActorID); listing is
// user-scoped for receivers, routes and silences, and unscoped for deliveries
// (an audit-oriented record).
type alertrouteHandler struct {
	service *alertroute.Service
}

// parseAlertRouteID parses a positive int64 path parameter such as ":id".
func parseAlertRouteID(c *gin.Context, param string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(param), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_ID", param+" must be a positive integer")
		return 0, false
	}
	return id, true
}

// --- Receivers ---

type receiverCreateRequest struct {
	Name   string `json:"name" binding:"required"`
	URL    string `json:"url" binding:"required"`
	Secret string `json:"secret" binding:"required"`
}

// listReceivers handles GET /api/v1/alert-routes/receivers.
func (h alertrouteHandler) listReceivers(c *gin.Context) {
	views, err := h.service.ListReceivers(c.Request.Context(), currentActorID(c))
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": views})
}

// createReceiver handles POST /api/v1/alert-routes/receivers.
func (h alertrouteHandler) createReceiver(c *gin.Context) {
	var request receiverCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	name := strings.TrimSpace(request.Name)
	receiver, err := h.service.CreateReceiver(c.Request.Context(), currentActorID(c), name, request.URL, request.Secret)
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	setAuditTarget(c, "AlertReceiver", "", strconv.FormatInt(receiver.ID, 10))
	c.JSON(http.StatusCreated, alertroute.ReceiverView{
		ID: receiver.ID, Name: receiver.Name, URLMasked: alertroute.MaskURL(receiver.URL), CreatorID: receiver.CreatorID,
	})
}

// deleteReceiver handles DELETE /api/v1/alert-routes/receivers/:id.
func (h alertrouteHandler) deleteReceiver(c *gin.Context) {
	id, ok := parseAlertRouteID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteReceiver(c.Request.Context(), id, currentActorID(c)); err != nil {
		writeAlertRouteError(c, err)
		return
	}
	setAuditTarget(c, "AlertReceiver", "", strconv.FormatInt(id, 10))
	c.Status(http.StatusNoContent)
}

// --- Routes ---

type routeCreateRequest struct {
	ReceiverID     int64  `json:"receiver_id" binding:"required"`
	Priority       int    `json:"priority"`
	ClusterID      *int64 `json:"cluster_id"`
	RuleName       string `json:"rule_name"`
	Severity       string `json:"severity"`
	DedupeKey      string `json:"dedupe_key"`
	GroupInterval  string `json:"group_interval"`
	RepeatInterval string `json:"repeat_interval"`
}

type routeUpdateRequest struct {
	Priority       *int    `json:"priority"`
	Enabled        *bool   `json:"enabled"`
	GroupInterval  *string `json:"group_interval"`
	RepeatInterval *string `json:"repeat_interval"`
}

// listRoutes handles GET /api/v1/alert-routes/.
func (h alertrouteHandler) listRoutes(c *gin.Context) {
	views, err := h.service.ListRoutes(c.Request.Context(), currentActorID(c))
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": views})
}

// createRoute handles POST /api/v1/alert-routes/.
func (h alertrouteHandler) createRoute(c *gin.Context) {
	var request routeCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	route := alertroute.Route{
		ReceiverID: request.ReceiverID,
		CreatorID:  currentActorID(c),
		Priority:   request.Priority,
		ClusterID:  request.ClusterID,
		RuleName:   request.RuleName,
		Severity:   request.Severity,
		DedupeKey:  request.DedupeKey,
	}
	if request.GroupInterval != "" {
		d, err := time.ParseDuration(request.GroupInterval)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "group_interval must be a valid duration")
			return
		}
		route.GroupInterval = &d
	}
	if request.RepeatInterval != "" {
		d, err := time.ParseDuration(request.RepeatInterval)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "repeat_interval must be a valid duration")
			return
		}
		route.RepeatInterval = &d
	}
	created, err := h.service.CreateRoute(c.Request.Context(), &route)
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	setAuditTarget(c, "AlertRoute", "", strconv.FormatInt(created.ID, 10))
	c.JSON(http.StatusCreated, routeViewFromRoute(created))
}

// updateRoute handles PATCH /api/v1/alert-routes/:id.
func (h alertrouteHandler) updateRoute(c *gin.Context) {
	id, ok := parseAlertRouteID(c, "id")
	if !ok {
		return
	}
	var request routeUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	input := alertroute.PatchRouteInput{
		Priority: request.Priority,
		Enabled:  request.Enabled,
	}
	if request.GroupInterval != nil {
		d, err := time.ParseDuration(*request.GroupInterval)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "group_interval must be a valid duration")
			return
		}
		input.GroupInterval = &d
	}
	if request.RepeatInterval != nil {
		d, err := time.ParseDuration(*request.RepeatInterval)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "repeat_interval must be a valid duration")
			return
		}
		input.RepeatInterval = &d
	}
	updated, err := h.service.UpdateRoute(c.Request.Context(), id, currentActorID(c), input)
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	setAuditTarget(c, "AlertRoute", "", strconv.FormatInt(id, 10))
	c.JSON(http.StatusOK, routeViewFromRoute(updated))
}

// deleteRoute handles DELETE /api/v1/alert-routes/:id.
func (h alertrouteHandler) deleteRoute(c *gin.Context) {
	id, ok := parseAlertRouteID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteRoute(c.Request.Context(), id, currentActorID(c)); err != nil {
		writeAlertRouteError(c, err)
		return
	}
	setAuditTarget(c, "AlertRoute", "", strconv.FormatInt(id, 10))
	c.Status(http.StatusNoContent)
}

// routeViewFromRoute projects a persistence-shaped Route into its public view.
// ReceiverName is not populated here; the list endpoint enriches it via the
// service. Create/update callers can fetch the list for the enriched view.
func routeViewFromRoute(route alertroute.Route) alertroute.RouteView {
	return alertroute.RouteView{
		ID:             route.ID,
		ReceiverID:     route.ReceiverID,
		Priority:       route.Priority,
		ClusterID:      route.ClusterID,
		RuleName:       route.RuleName,
		Severity:       route.Severity,
		DedupeKey:      route.DedupeKey,
		GroupInterval:  route.GroupInterval,
		RepeatInterval: route.RepeatInterval,
		Enabled:        route.Enabled,
	}
}

// --- Silences ---

type silenceCreateRequest struct {
	ClusterID *int64 `json:"cluster_id"`
	RuleName  string `json:"rule_name"`
	Severity  string `json:"severity"`
	Reason    string `json:"reason" binding:"required"`
	StartsAt  string `json:"starts_at" binding:"required"`
	EndsAt    string `json:"ends_at" binding:"required"`
}

// listSilences handles GET /api/v1/alert-routes/silences. The optional
// "active" query param filters to currently-active silences when "true".
func (h alertrouteHandler) listSilences(c *gin.Context) {
	creatorID := currentActorID(c)
	filter := alertroute.SilenceListFilter{CreatorID: &creatorID}
	if raw := c.Query("active"); raw != "" {
		active, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "active must be true or false")
			return
		}
		filter.Active = &active
	}
	views, err := h.service.ListSilences(c.Request.Context(), filter)
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": views})
}

// createSilence handles POST /api/v1/alert-routes/silences.
func (h alertrouteHandler) createSilence(c *gin.Context) {
	var request silenceCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	startsAt, err := time.Parse(time.RFC3339Nano, request.StartsAt)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "starts_at must be an RFC3339 timestamp")
		return
	}
	endsAt, err := time.Parse(time.RFC3339Nano, request.EndsAt)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "ends_at must be an RFC3339 timestamp")
		return
	}
	silence := alertroute.Silence{
		CreatorID: currentActorID(c),
		ClusterID: request.ClusterID,
		RuleName:  request.RuleName,
		Severity:  request.Severity,
		Reason:    strings.TrimSpace(request.Reason),
		StartsAt:  startsAt,
		EndsAt:    endsAt,
	}
	created, err := h.service.CreateSilence(c.Request.Context(), &silence)
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	setAuditTarget(c, "AlertSilence", "", strconv.FormatInt(created.ID, 10))
	c.JSON(http.StatusCreated, silenceViewFromSilence(created))
}

// deleteSilence handles DELETE /api/v1/alert-routes/silences/:id.
func (h alertrouteHandler) deleteSilence(c *gin.Context) {
	id, ok := parseAlertRouteID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteSilence(c.Request.Context(), id, currentActorID(c)); err != nil {
		writeAlertRouteError(c, err)
		return
	}
	setAuditTarget(c, "AlertSilence", "", strconv.FormatInt(id, 10))
	c.Status(http.StatusNoContent)
}

func silenceViewFromSilence(s alertroute.Silence) alertroute.SilenceView {
	return alertroute.SilenceView{
		ID: s.ID, ClusterID: s.ClusterID, RuleName: s.RuleName, Severity: s.Severity,
		Reason: s.Reason, StartsAt: s.StartsAt, EndsAt: s.EndsAt, CreatorID: s.CreatorID,
	}
}

// --- Inhibits (M51) ---

type inhibitCreateRequest struct {
	SourceClusterID *int64 `json:"source_cluster_id"`
	SourceRuleName  string `json:"source_rule_name"`
	SourceSeverity  string `json:"source_severity"`
	TargetClusterID *int64 `json:"target_cluster_id"`
	TargetRuleName  string `json:"target_rule_name"`
	TargetSeverity  string `json:"target_severity"`
	Reason          string `json:"reason" binding:"required"`
}

// listInhibits handles GET /api/v1/alert-routes/inhibits. The optional
// "enabled" query param filters to enabled/disabled inhibits when "true"/"false".
func (h alertrouteHandler) listInhibits(c *gin.Context) {
	creatorID := currentActorID(c)
	filter := alertroute.InhibitListFilter{CreatorID: &creatorID}
	if raw := c.Query("enabled"); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "enabled must be true or false")
			return
		}
		filter.Enabled = &enabled
	}
	views, err := h.service.ListInhibits(c.Request.Context(), filter)
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": views})
}

// createInhibit handles POST /api/v1/alert-routes/inhibits.
func (h alertrouteHandler) createInhibit(c *gin.Context) {
	var request inhibitCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	inhibit := alertroute.Inhibit{
		CreatorID:       currentActorID(c),
		SourceClusterID: request.SourceClusterID,
		SourceRuleName:  request.SourceRuleName,
		SourceSeverity:  request.SourceSeverity,
		TargetClusterID: request.TargetClusterID,
		TargetRuleName:  request.TargetRuleName,
		TargetSeverity:  request.TargetSeverity,
		Reason:          strings.TrimSpace(request.Reason),
	}
	created, err := h.service.CreateInhibit(c.Request.Context(), &inhibit)
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	setAuditTarget(c, "AlertInhibit", "", strconv.FormatInt(created.ID, 10))
	c.JSON(http.StatusCreated, inhibitViewFromInhibit(created))
}

// deleteInhibit handles DELETE /api/v1/alert-routes/inhibits/:id.
func (h alertrouteHandler) deleteInhibit(c *gin.Context) {
	id, ok := parseAlertRouteID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteInhibit(c.Request.Context(), id, currentActorID(c)); err != nil {
		writeAlertRouteError(c, err)
		return
	}
	setAuditTarget(c, "AlertInhibit", "", strconv.FormatInt(id, 10))
	c.Status(http.StatusNoContent)
}

func inhibitViewFromInhibit(i alertroute.Inhibit) alertroute.InhibitView {
	return alertroute.InhibitView{
		ID: i.ID, SourceClusterID: i.SourceClusterID, SourceRuleName: i.SourceRuleName,
		SourceSeverity: i.SourceSeverity, TargetClusterID: i.TargetClusterID,
		TargetRuleName: i.TargetRuleName, TargetSeverity: i.TargetSeverity,
		Reason: i.Reason, Enabled: i.Enabled, CreatorID: i.CreatorID,
	}
}

// --- Deliveries ---

// listDeliveries handles GET /api/v1/alert-routes/deliveries. The optional
// "status" query param filters by delivery status (pending, delivering,
// delivered, dead).
func (h alertrouteHandler) listDeliveries(c *gin.Context) {
	filter := alertroute.DeliveryListFilter{Status: c.Query("status")}
	response, err := h.service.ListDeliveries(c.Request.Context(), filter)
	if err != nil {
		writeAlertRouteError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

// writeAlertRouteError maps alertroute service errors to stable HTTP responses.
// Validation errors return 400, not-found (user-scoped) return 404, limit and
// conflict errors return 409, and anything else is a 500 with a sanitized
// message that never leaks provider detail.
func writeAlertRouteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, alertroute.ErrReceiverNotFound):
		writeError(c, http.StatusNotFound, "RECEIVER_NOT_FOUND", "alert receiver not found")
	case errors.Is(err, alertroute.ErrReceiverInUse):
		writeError(c, http.StatusConflict, "RECEIVER_IN_USE", "receiver is referenced by routes")
	case errors.Is(err, alertroute.ErrRouteNotFound):
		writeError(c, http.StatusNotFound, "ROUTE_NOT_FOUND", "alert route not found")
	case errors.Is(err, alertroute.ErrSilenceNotFound):
		writeError(c, http.StatusNotFound, "SILENCE_NOT_FOUND", "alert silence not found")
	case errors.Is(err, alertroute.ErrInhibitNotFound):
		writeError(c, http.StatusNotFound, "INHIBIT_NOT_FOUND", "alert inhibit not found")
	case errors.Is(err, alertroute.ErrDeliveryNotFound):
		writeError(c, http.StatusNotFound, "DELIVERY_NOT_FOUND", "alert route delivery not found")
	case errors.Is(err, alertroute.ErrInvalidReceiver):
		writeError(c, http.StatusBadRequest, "INVALID_RECEIVER", "alert receiver fields are invalid")
	case errors.Is(err, alertroute.ErrInvalidRoute):
		writeError(c, http.StatusBadRequest, "INVALID_ROUTE", "alert route fields are invalid")
	case errors.Is(err, alertroute.ErrInvalidSilence):
		writeError(c, http.StatusBadRequest, "INVALID_SILENCE", "alert silence fields are invalid")
	case errors.Is(err, alertroute.ErrInvalidInhibit):
		writeError(c, http.StatusBadRequest, "INVALID_INHIBIT", "alert inhibit fields are invalid")
	case errors.Is(err, alertroute.ErrPermanentSilence):
		writeError(c, http.StatusBadRequest, "SILENCE_TOO_LONG", "silence duration exceeds the maximum")
	case errors.Is(err, alertroute.ErrSilenceExpired):
		writeError(c, http.StatusBadRequest, "SILENCE_EXPIRED", "silence end time must be in the future")
	case errors.Is(err, alertroute.ErrReceiverLimit):
		writeError(c, http.StatusConflict, "RECEIVER_LIMIT_REACHED", "receiver limit reached for user")
	case errors.Is(err, alertroute.ErrRouteLimit):
		writeError(c, http.StatusConflict, "ROUTE_LIMIT_REACHED", "route limit reached for user")
	case errors.Is(err, alertroute.ErrSilenceLimit):
		writeError(c, http.StatusConflict, "SILENCE_LIMIT_REACHED", "silence limit reached for user")
	case errors.Is(err, alertroute.ErrInhibitLimit):
		writeError(c, http.StatusConflict, "INHIBIT_LIMIT_REACHED", "inhibit limit reached for user")
	case errors.Is(err, alertroute.ErrDuplicateReceiverName):
		writeError(c, http.StatusConflict, "RECEIVER_NAME_EXISTS", "receiver name already exists for user")
	default:
		writeError(c, http.StatusInternalServerError, "ALERT_ROUTE_FAILED", "unable to manage alert routes")
	}
}
