package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/incident"
	"k8s-aiops.local/backend/internal/incidentchat"
)

// incidentChatAdapter adapts incident.Service to the chat service's
// IncidentReader interface by projecting the incident snapshot and its
// evidence timeline.
type incidentChatAdapter struct {
	service *incident.Service
}

func (a incidentChatAdapter) Get(ctx context.Context, id int64) (incidentchat.IncidentSnapshot, []incidentchat.EvidenceItem, error) {
	rec, err := a.service.Get(ctx, id)
	if err != nil {
		return incidentchat.IncidentSnapshot{}, nil, err
	}
	snap := incidentchat.IncidentSnapshot{
		ID:         rec.ID,
		Number:     rec.Number,
		Title:      rec.Title,
		Severity:   rec.Severity,
		Status:     rec.Status,
		Summary:    rec.Summary,
		SourceType: rec.SourceType,
		ClusterID:  rec.ClusterID,
		Namespace:  rec.Resource.Namespace,
		Kind:       rec.Resource.Kind,
		Name:       rec.Resource.Name,
	}
	evidence, err := a.service.Evidence(ctx, id)
	if err != nil {
		return snap, nil, err
	}
	items := make([]incidentchat.EvidenceItem, 0, len(evidence))
	for _, e := range evidence {
		fields := make([]incidentchat.Field, 0, len(e.Fields))
		for _, f := range e.Fields {
			fields = append(fields, incidentchat.Field{Label: f.Label, Value: f.Value})
		}
		items = append(items, incidentchat.EvidenceItem{
			SourceType: e.SourceType,
			SourceRef:  e.SourceRef,
			Title:      e.Title,
			Summary:    e.Summary,
			Severity:   e.Severity,
			ObservedAt: e.ObservedAt,
			DeepLink:   e.DeepLink,
			Fields:     fields,
		})
	}
	return snap, items, nil
}

type incidentChatHandler struct {
	service *incidentchat.Service
	adapter incidentchat.IncidentReader
}

type incidentChatRequest struct {
	Messages []incidentchat.ChatMessage `json:"messages" binding:"required"`
}

func (h incidentChatHandler) chat(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("incident_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_INCIDENT_ID", "incident_id must be a positive integer")
		return
	}
	setAuditTarget(c, "Incident", "", strconv.FormatInt(id, 10))
	var request incidentChatRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "messages are required")
		return
	}
	if len(request.Messages) == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "at least one message is required")
		return
	}
	// Trim whitespace and reject history that only carries the last user turn.
	var normalized []incidentchat.ChatMessage
	for _, m := range request.Messages {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		normalized = append(normalized, incidentchat.ChatMessage{Role: strings.TrimSpace(m.Role), Content: content})
	}
	if len(normalized) == 0 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "message content must not be empty")
		return
	}

	// Final incident existence check before exposing the chat surface.
	response, err := h.service.Chat(c.Request.Context(), id, normalized, time.Now().UTC())
	switch {
	case errors.Is(err, incident.ErrNotFound):
		writeError(c, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident does not exist")
	case errors.Is(err, incidentchat.ErrNoMessages), errors.Is(err, incidentchat.ErrLastMessageNotUser), errors.Is(err, incidentchat.ErrHistoryTooLong):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	case errors.Is(err, incidentchat.ErrBusy):
		writeError(c, http.StatusServiceUnavailable, "AI_CHAT_BUSY", "incident AI chat concurrency limit reached")
	case err != nil:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to process incident chat")
	default:
		c.JSON(http.StatusOK, response)
	}
}