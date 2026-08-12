package httpserver

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/incident"
	"k8s-aiops.local/backend/internal/requestctx"
)

type incidentHandler struct {
	service *incident.Service
}

type createIncidentRequest struct {
	SourceType string               `json:"source_type" binding:"required"`
	SourceRef  string               `json:"source_ref" binding:"required"`
	ClusterID  int64                `json:"cluster_id" binding:"required"`
	Title      string               `json:"title"`
	Severity   string               `json:"severity"`
	Summary    string               `json:"summary"`
	ObservedAt string               `json:"observed_at"`
	Resource   incident.ResourceRef `json:"resource"`
}

type incidentTransitionRequest struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required"`
	Status          string `json:"status" binding:"required"`
	Comment         string `json:"comment"`
}

type incidentAssignmentRequest struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required"`
	AssigneeUserID  int64  `json:"assignee_user_id" binding:"required"`
	Comment         string `json:"comment"`
}

type incidentNoteRequest struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required"`
	Content         string `json:"content" binding:"required"`
}

type incidentPostmortemRequest struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required"`
	Content         string `json:"content"`
}

type incidentFollowerRequest struct {
	UserID int64 `json:"user_id" binding:"required"`
}

func incidentID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("incident_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_INCIDENT_ID", "incident_id must be a positive integer")
		return 0, false
	}
	return id, true
}

func incidentActor(c *gin.Context) incident.ActorRef {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	return incident.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName}
}

func (h incidentHandler) create(c *gin.Context) {
	var request createIncidentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "source_type, source_ref and cluster_id are required")
		return
	}
	request.SourceType = strings.TrimSpace(request.SourceType)
	request.SourceRef = strings.TrimSpace(request.SourceRef)
	if len(request.Summary) > 4000 || len(request.Title) > 500 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "title must not exceed 500 and summary must not exceed 4000 characters")
		return
	}
	if request.Resource.Kind != "" {
		setAuditTarget(c, request.Resource.Kind, request.Resource.Namespace, request.Resource.Name)
	}
	var observedAt time.Time
	if request.ObservedAt != "" {
		parsed, err := time.Parse(time.RFC3339, request.ObservedAt)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "observed_at must be an RFC3339 timestamp")
			return
		}
		observedAt = parsed
	}
	record, err := h.service.Create(c.Request.Context(), incident.CreateInput{
		SourceType: request.SourceType,
		SourceRef:  request.SourceRef,
		ClusterID:  request.ClusterID,
		Title:      request.Title,
		Severity:   request.Severity,
		Summary:    request.Summary,
		ObservedAt: observedAt,
		Resource:   request.Resource,
	})
	if err == nil {
		c.JSON(http.StatusCreated, record)
		return
	}
	switch {
	case errors.Is(err, incident.ErrSourceAlreadyUsed):
		writeError(c, http.StatusConflict, "SOURCE_ALREADY_USED", "this diagnosis or finding already has an incident workspace")
	case errors.Is(err, incident.ErrInvalidSource), errors.Is(err, incident.ErrInvalidTitle):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "source_type, source_ref, cluster_id, severity and resource identity are required")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to create incident")
	}
}

func (h incidentHandler) get(c *gin.Context) {
	id, ok := incidentID(c)
	if !ok {
		return
	}
	record, err := h.service.Get(c.Request.Context(), id)
	if err == nil {
		c.JSON(http.StatusOK, record)
		return
	}
	if errors.Is(err, incident.ErrNotFound) {
		writeError(c, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident does not exist")
		return
	}
	writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to read incident")
}

func (h incidentHandler) list(c *gin.Context) {
	clusterID, err := strconv.ParseInt(defaultString(c.Query("cluster_id"), "0"), 10, 64)
	if err != nil || clusterID < 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
		return
	}
	limit, err := strconv.Atoi(defaultString(c.Query("limit"), "50"))
	if err != nil || limit < 1 || limit > 200 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be between 1 and 200")
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && status != incident.StatusOpen && status != incident.StatusConfirmed && status != incident.StatusResolved && status != incident.StatusDismissed {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "status must be open, confirmed, resolved or dismissed")
		return
	}
	assigneeID, err := strconv.ParseInt(defaultString(c.Query("assignee_id"), "0"), 10, 64)
	if err != nil || assigneeID < 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "assignee_id must be a positive integer")
		return
	}
	followerID, err := strconv.ParseInt(defaultString(c.Query("follower_id"), "0"), 10, 64)
	if err != nil || followerID < 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "follower_id must be a positive integer")
		return
	}
	items, err := h.service.List(c.Request.Context(), incident.ListFilter{
		ClusterID: clusterID, Status: status, AssigneeID: assigneeID, FollowerID: followerID, Limit: limit,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list incidents")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "remaining": 0})
}

func (h incidentHandler) summary(c *gin.Context) {
	summary, err := h.service.Summary(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to summarize incidents")
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (h incidentHandler) transition(c *gin.Context) {
	id, ok := incidentID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Incident", "", strconv.FormatInt(id, 10))
	var request incidentTransitionRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Comment) > 2000 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "status and expected_version are required and comment must not exceed 2000 characters")
		return
	}
	record, err := h.service.Transition(c.Request.Context(), id, request.ExpectedVersion, strings.TrimSpace(request.Status), incidentActor(c), request.Comment)
	if err == nil {
		c.JSON(http.StatusOK, record)
		return
	}
	switch {
	case errors.Is(err, incident.ErrNotFound):
		writeError(c, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident does not exist")
	case errors.Is(err, incident.ErrVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "expected_version is stale; refresh the incident and retry")
	case errors.Is(err, incident.ErrInvalidTransition):
		writeError(c, http.StatusConflict, "INVALID_STATUS_TRANSITION", "the requested incident status transition is not allowed")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to update incident")
	}
}

func (h incidentHandler) assign(c *gin.Context) {
	id, ok := incidentID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Incident", "", strconv.FormatInt(id, 10))
	var request incidentAssignmentRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.AssigneeUserID < 1 || len(request.Comment) > 2000 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "assignee_user_id and expected_version are required and comment must not exceed 2000 characters")
		return
	}
	record, err := h.service.Assign(c.Request.Context(), id, request.ExpectedVersion, request.AssigneeUserID, incidentActor(c), request.Comment)
	if err == nil {
		c.JSON(http.StatusOK, record)
		return
	}
	switch {
	case errors.Is(err, incident.ErrNotFound):
		writeError(c, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident does not exist")
	case errors.Is(err, incident.ErrVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "expected_version is stale; refresh the incident and retry")
	case errors.Is(err, incident.ErrAssigneeNotFound):
		writeError(c, http.StatusBadRequest, "ASSIGNEE_NOT_FOUND", "assignee user does not exist")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to assign incident")
	}
}

func (h incidentHandler) addFollower(c *gin.Context) {
	id, ok := incidentID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Incident", "", strconv.FormatInt(id, 10))
	var request incidentFollowerRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.UserID < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "user_id is required")
		return
	}
	record, err := h.service.AddFollower(c.Request.Context(), id, request.UserID, incidentActor(c))
	if err == nil {
		c.JSON(http.StatusCreated, record)
		return
	}
	switch {
	case errors.Is(err, incident.ErrNotFound):
		writeError(c, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident does not exist")
	case errors.Is(err, incident.ErrFollowerDuplicate):
		writeError(c, http.StatusConflict, "ALREADY_FOLLOWING", "user already follows this incident")
	case errors.Is(err, incident.ErrAssigneeNotFound):
		writeError(c, http.StatusNotFound, "INCIDENT_USER_NOT_FOUND", "user does not exist")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to add follower")
	}
}

func (h incidentHandler) removeFollower(c *gin.Context) {
	id, ok := incidentID(c)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || userID < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_USER_ID", "user_id must be a positive integer")
		return
	}
	setAuditTarget(c, "Incident", "", strconv.FormatInt(id, 10))
	record, err := h.service.RemoveFollower(c.Request.Context(), id, userID, incidentActor(c))
	if err == nil {
		c.JSON(http.StatusOK, record)
		return
	}
	switch {
	case errors.Is(err, incident.ErrNotFound):
		writeError(c, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident does not exist")
	case errors.Is(err, incident.ErrFollowerNotFound):
		writeError(c, http.StatusNotFound, "FOLLOWER_NOT_FOUND", "user does not follow this incident")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to remove follower")
	}
}

func (h incidentHandler) addNote(c *gin.Context) {
	id, ok := incidentID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Incident", "", strconv.FormatInt(id, 10))
	var request incidentNoteRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Content) > 4000 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "content and expected_version are required and content must not exceed 4000 characters")
		return
	}
	record, err := h.service.AddNote(c.Request.Context(), id, request.ExpectedVersion, incidentActor(c), request.Content)
	if err == nil {
		c.JSON(http.StatusCreated, record)
		return
	}
	switch {
	case errors.Is(err, incident.ErrNotFound):
		writeError(c, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident does not exist")
	case errors.Is(err, incident.ErrVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "expected_version is stale; refresh the incident and retry")
	case errors.Is(err, incident.ErrInvalidNote):
		writeError(c, http.StatusBadRequest, "INVALID_NOTE", "note content is required")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to add note")
	}
}

func (h incidentHandler) setPostmortem(c *gin.Context) {
	id, ok := incidentID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Incident", "", strconv.FormatInt(id, 10))
	var request incidentPostmortemRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Content) > 10000 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "expected_version is required and content must not exceed 10000 characters")
		return
	}
	record, err := h.service.SetPostmortem(c.Request.Context(), id, request.ExpectedVersion, incidentActor(c), request.Content)
	if err == nil {
		c.JSON(http.StatusOK, record)
		return
	}
	switch {
	case errors.Is(err, incident.ErrNotFound):
		writeError(c, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident does not exist")
	case errors.Is(err, incident.ErrVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "expected_version is stale; refresh the incident and retry")
	case errors.Is(err, incident.ErrPostmortemLocked):
		writeError(c, http.StatusConflict, "POSTMORTEM_LOCKED", "postmortem is only writable for resolved incidents")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to update postmortem")
	}
}

func (h incidentHandler) export(c *gin.Context) {
	id, ok := incidentID(c)
	if !ok {
		return
	}
	record, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, incident.ErrNotFound) {
			writeError(c, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident does not exist")
			return
		}
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to read incident")
		return
	}
	setAuditTarget(c, "IncidentExport", "", strconv.FormatInt(id, 10))
	var buffer bytes.Buffer
	result, err := h.service.ExportOne(c.Request.Context(), id, &buffer)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to export incident")
		return
	}
	filename := "incident-" + record.Number + "-" + time.Now().UTC().Format("20060102-150405Z") + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Incident-Export-Rows", strconv.Itoa(result.Rows))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buffer.Bytes())
}
