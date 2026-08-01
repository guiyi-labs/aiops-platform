package httpserver

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/audit"
	"k8s-aiops.local/backend/internal/requestctx"
)

type auditRecorder interface {
	Record(*gin.Context, *audit.Entry) error
}

type auditServiceAdapter struct{ service *audit.Service }

func (a auditServiceAdapter) Record(c *gin.Context, entry *audit.Entry) error {
	return a.service.Record(c.Request.Context(), entry)
}

type auditHandler struct{ service *audit.Service }

func (h auditHandler) list(c *gin.Context) {
	filter, ok := parseAuditFilter(c, 50, 100)
	if !ok {
		return
	}
	response, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list audit logs")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h auditHandler) export(c *gin.Context) {
	filter, ok := parseAuditFilter(c, 5000, 5000)
	if !ok {
		return
	}
	var buffer bytes.Buffer
	result, err := h.service.ExportCSV(c.Request.Context(), filter, &buffer)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to export audit logs")
		return
	}
	filename := "audit-logs-" + time.Now().UTC().Format("20060102-150405Z") + ".csv"
	setAuditTarget(c, "AuditExport", "", filename)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Audit-Export-Rows", strconv.Itoa(result.Rows))
	c.Header("X-Audit-Export-Total", strconv.FormatInt(result.Total, 10))
	c.Header("X-Audit-Export-Truncated", strconv.FormatBool(result.Truncated))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buffer.Bytes())
}

func parseAuditFilter(c *gin.Context, defaultLimit, maxLimit int) (audit.Filter, bool) {
	clusterID, err := strconv.ParseInt(defaultString(c.Query("cluster_id"), "0"), 10, 64)
	if err != nil || clusterID < 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
		return audit.Filter{}, false
	}
	action := strings.TrimSpace(c.Query("action"))
	if len(action) > 128 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "action must not exceed 128 characters")
		return audit.Filter{}, false
	}
	result := strings.TrimSpace(c.Query("result"))
	if result != "" && result != "success" && result != "failure" && result != "denied" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "result must be success, failure or denied")
		return audit.Filter{}, false
	}
	limit, err := strconv.Atoi(defaultString(c.Query("limit"), strconv.Itoa(defaultLimit)))
	if err != nil || limit < 1 || limit > maxLimit {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", fmt.Sprintf("limit must be between 1 and %d", maxLimit))
		return audit.Filter{}, false
	}
	return audit.Filter{ClusterID: clusterID, Action: action, Result: result, Limit: limit}, true
}

func auditTrail(logger *zap.Logger, recorder auditRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		action, fallbackResource, ok := findAuditedRoute(c.Request.Method, c.FullPath())
		if !ok {
			return
		}
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		resourceType := metadata.Resource
		if resourceType == "" {
			resourceType = fallbackResource
		}
		resourceName := metadata.Name
		if resourceName == "" {
			resourceName = firstNonEmpty(c.Param("diagnosis_id"), c.Param("cluster_id"), c.Param("rule_id"), c.Param("alert_id"))
		}
		var clusterID *int64
		value := metadata.ClusterID
		if value == 0 {
			value, _ = strconv.ParseInt(c.Param("cluster_id"), 10, 64)
		}
		if value > 0 {
			clusterID = &value
		}
		if action == "cluster.delete" {
			clusterID = nil
		}
		entry := &audit.Entry{
			Actor:     audit.Actor{ID: metadata.ActorID, Name: firstNonEmpty(metadata.ActorDisplayName, metadata.ActorName)},
			ClusterID: clusterID, Action: action,
			Resource: audit.ResourceRef{Type: resourceType, Namespace: metadata.Namespace, Name: resourceName},
			Result:   auditResult(c.Writer.Status()), RequestID: metadata.RequestID, StatusCode: c.Writer.Status(),
			IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent(),
			Details: map[string]any{"method": c.Request.Method, "path_template": c.FullPath(), "cluster_id": value},
		}
		if err := recorder.Record(c, entry); err != nil {
			logger.Error("write audit log", zap.Error(err), zap.String("request_id", metadata.RequestID), zap.String("action", action))
		}
	}
}

// auditedOperation is retained as a thin shim over the descriptor-driven
// routeTable so existing call sites and tests continue to work while the
// hardcoded map has been removed. New code should call findAuditedRoute
// directly.
func auditedOperation(method, path string) (string, string, bool) {
	return findAuditedRoute(method, path)
}

func auditResult(status int) string {
	if status >= 200 && status < 300 {
		return "success"
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "denied"
	}
	return "failure"
}

func setAuditTarget(c *gin.Context, resource, namespace, name string) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	metadata.Resource, metadata.Namespace, metadata.Name = resource, namespace, name
	c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
}

func setAuditActor(c *gin.Context, id int64, username, displayName string, roles []string) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	metadata.ActorID, metadata.ActorName, metadata.ActorDisplayName = id, username, displayName
	metadata.Roles = append([]string(nil), roles...)
	c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
}

func setAuditClusterID(c *gin.Context, id int64) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	metadata.ClusterID = id
	c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
