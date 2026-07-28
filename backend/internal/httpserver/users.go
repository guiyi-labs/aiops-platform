package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/requestctx"
)

type userHandler struct{ service *auth.Service }

type createUserRequest struct {
	Username    string   `json:"username" binding:"required"`
	Password    string   `json:"password" binding:"required"`
	DisplayName string   `json:"display_name" binding:"required"`
	Roles       []string `json:"roles" binding:"required"`
}

type updateUserRequest struct {
	DisplayName *string   `json:"display_name"`
	Status      *string   `json:"status"`
	Roles       *[]string `json:"roles"`
}

type resetUserPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

func (h userHandler) assignable(c *gin.Context) {
	items, err := h.service.AssignableUsers(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list assignable users")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "remaining": 0})
}

func (h userHandler) list(c *gin.Context) {
	limit, err := strconv.Atoi(defaultString(c.Query("limit"), "100"))
	if err != nil || limit < 1 || limit > 100 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be between 1 and 100")
		return
	}
	items, total, err := h.service.ListUsers(c.Request.Context(), limit)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list users")
		return
	}
	remaining := total - int64(len(items))
	if remaining < 0 {
		remaining = 0
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "remaining": remaining})
}

func (h userHandler) create(c *gin.Context) {
	var request createUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "username, password, display_name and roles are required")
		return
	}
	setAuditTarget(c, "User", "", strings.TrimSpace(request.Username))
	user, err := h.service.CreateUser(c.Request.Context(), auth.CreateUserInput{Username: request.Username, Password: request.Password, DisplayName: request.DisplayName, Roles: request.Roles})
	if err == nil {
		c.JSON(http.StatusCreated, user)
		return
	}
	switch {
	case errors.Is(err, auth.ErrInvalidUser):
		writeError(c, http.StatusBadRequest, "INVALID_USER", "username must be lowercase and safe, password at least 12 characters, display_name and at least one valid role are required")
	case errors.Is(err, auth.ErrUsernameExists):
		writeError(c, http.StatusConflict, "USERNAME_EXISTS", "username already exists")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to create user")
	}
}

func (h userHandler) update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_USER_ID", "user_id must be a positive integer")
		return
	}
	setAuditTarget(c, "User", "", strconv.FormatInt(id, 10))
	var request updateUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "display_name, status or roles must be supplied")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	user, err := h.service.UpdateUser(c.Request.Context(), id, metadata.ActorID, auth.UpdateUserInput{DisplayName: request.DisplayName, Status: request.Status, Roles: request.Roles})
	if err == nil {
		setAuditTarget(c, "User", "", user.Username)
		c.JSON(http.StatusOK, user)
		return
	}
	switch {
	case errors.Is(err, auth.ErrInvalidUser):
		writeError(c, http.StatusBadRequest, "INVALID_USER", "display_name, status or roles are invalid")
	case errors.Is(err, auth.ErrUserNotFound):
		writeError(c, http.StatusNotFound, "USER_NOT_FOUND", "user does not exist")
	case errors.Is(err, auth.ErrSelfProtection):
		writeError(c, http.StatusConflict, "SELF_PROTECTION", "the current user cannot disable itself or change its own roles")
	case errors.Is(err, auth.ErrLastSystemAdmin):
		writeError(c, http.StatusConflict, "LAST_SYSTEM_ADMIN", "at least one active system administrator must remain")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to update user")
	}
}

func (h userHandler) resetPassword(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_USER_ID", "user_id must be a positive integer")
		return
	}
	setAuditTarget(c, "User", "", strconv.FormatInt(id, 10))
	var request resetUserPasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "password is required")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	user, err := h.service.ResetPassword(c.Request.Context(), id, metadata.ActorID, request.Password)
	if err == nil {
		setAuditTarget(c, "User", "", user.Username)
		c.JSON(http.StatusOK, user)
		return
	}
	switch {
	case errors.Is(err, auth.ErrInvalidUser):
		writeError(c, http.StatusBadRequest, "INVALID_PASSWORD", "password must be between 12 and 128 characters")
	case errors.Is(err, auth.ErrUserNotFound):
		writeError(c, http.StatusNotFound, "USER_NOT_FOUND", "user does not exist")
	case errors.Is(err, auth.ErrSelfPasswordReset):
		writeError(c, http.StatusConflict, "SELF_PASSWORD_RESET", "administrators cannot reset their own password from user management")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to reset user password")
	}
}
