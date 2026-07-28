package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/notification"
)

type notificationHandler struct{ service *notification.Service }

func (h notificationHandler) list(c *gin.Context) {
	diagnosisID, err := strconv.ParseInt(defaultString(c.Query("diagnosis_id"), "0"), 10, 64)
	if err != nil || diagnosisID < 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "diagnosis_id must be a positive integer")
		return
	}
	eventType := strings.TrimSpace(c.Query("event_type"))
	if eventType != "" && eventType != "diagnosis.created" && eventType != "diagnosis.status_changed" && eventType != "diagnosis.assigned" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "event_type is not supported")
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && status != "pending" && status != "delivering" && status != "delivered" && status != "dead" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "status must be pending, delivering, delivered or dead")
		return
	}
	limit, err := strconv.Atoi(defaultString(c.Query("limit"), "50"))
	if err != nil || limit < 1 || limit > 100 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be between 1 and 100")
		return
	}
	response, err := h.service.List(c.Request.Context(), notification.ListFilter{DiagnosisID: diagnosisID, EventType: eventType, Status: status, Limit: limit})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list notification deliveries")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h notificationHandler) retry(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("delivery_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_DELIVERY_ID", "delivery_id must be a positive integer")
		return
	}
	setAuditTarget(c, "NotificationDelivery", "", strconv.FormatInt(id, 10))
	if !h.service.Enabled() {
		writeError(c, http.StatusConflict, "NOTIFICATIONS_DISABLED", "notification delivery is not enabled")
		return
	}
	err = h.service.Retry(c.Request.Context(), id)
	switch {
	case err == nil:
		c.Status(http.StatusAccepted)
	case errors.Is(err, notification.ErrDeliveryNotFound):
		writeError(c, http.StatusNotFound, "DELIVERY_NOT_FOUND", "notification delivery does not exist")
	case errors.Is(err, notification.ErrDeliveryNotRetryable):
		writeError(c, http.StatusConflict, "DELIVERY_NOT_RETRYABLE", "only dead notification deliveries can be retried")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to retry notification delivery")
	}
}
