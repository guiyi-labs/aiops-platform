package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/globalsearch"
	"k8s-aiops.local/backend/internal/requestctx"
)

type savedGlobalSearchFilterHandler struct {
	service *globalsearch.SavedFilterService
}

type savedFilterCreateRequest struct {
	Name      string              `json:"name"`
	Query     string              `json:"query"`
	Namespace string              `json:"namespace"`
	Kinds     []globalsearch.Kind `json:"kinds"`
}

type savedFilterUpdateRequest struct {
	Name      *string              `json:"name"`
	Query     *string              `json:"query"`
	Namespace *string              `json:"namespace"`
	Kinds     *[]globalsearch.Kind `json:"kinds"`
}

func (h savedGlobalSearchFilterHandler) list(c *gin.Context) {
	response, err := h.service.List(c.Request.Context(), currentActorID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h savedGlobalSearchFilterHandler) create(c *gin.Context) {
	var request savedFilterCreateRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_SAVED_FILTER", "request must contain only the saved filter fields")
		return
	}
	item, err := h.service.Create(c.Request.Context(), currentActorID(c), globalsearch.CreateSavedFilterInput{
		Name: request.Name, Query: request.Query, Namespace: request.Namespace, Kinds: request.Kinds,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	setAuditTarget(c, "GlobalSearchFilter", "", strconv.FormatInt(item.ID, 10))
	c.JSON(http.StatusCreated, item)
}

func (h savedGlobalSearchFilterHandler) update(c *gin.Context) {
	id, ok := savedFilterID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "GlobalSearchFilter", "", strconv.FormatInt(id, 10))
	var request savedFilterUpdateRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_SAVED_FILTER", "request must contain only the saved filter fields")
		return
	}
	item, err := h.service.Update(c.Request.Context(), currentActorID(c), id, globalsearch.UpdateSavedFilterInput{
		Name: request.Name, Query: request.Query, Namespace: request.Namespace, Kinds: request.Kinds,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h savedGlobalSearchFilterHandler) delete(c *gin.Context) {
	id, ok := savedFilterID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "GlobalSearchFilter", "", strconv.FormatInt(id, 10))
	if err := h.service.Delete(c.Request.Context(), currentActorID(c), id); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (savedGlobalSearchFilterHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, globalsearch.ErrInvalidSavedFilter):
		writeError(c, http.StatusBadRequest, "INVALID_SAVED_FILTER", "saved filter fields are invalid")
	case errors.Is(err, globalsearch.ErrSavedFilterNotFound):
		writeError(c, http.StatusNotFound, "SAVED_FILTER_NOT_FOUND", "saved filter does not exist")
	case errors.Is(err, globalsearch.ErrSavedFilterLimit):
		writeError(c, http.StatusConflict, "SAVED_FILTER_LIMIT_REACHED", "saved filter limit has been reached")
	case errors.Is(err, globalsearch.ErrSavedFilterNameExists):
		writeError(c, http.StatusConflict, "SAVED_FILTER_NAME_EXISTS", "saved filter name already exists")
	default:
		writeError(c, http.StatusInternalServerError, "SAVED_FILTERS_FAILED", "unable to manage saved filters")
	}
}

func currentActorID(c *gin.Context) int64 {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	return metadata.ActorID
}

func savedFilterID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("filter_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_SAVED_FILTER_ID", "filter_id must be a positive integer")
		return 0, false
	}
	return id, true
}
