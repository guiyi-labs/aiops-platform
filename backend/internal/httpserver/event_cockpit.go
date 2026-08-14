package httpserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/eventcockpit"
)

// eventCockpit is an M114-2 endpoint on kubernetesHandler: a
// read-only aggregation of Kubernetes Events grouped by severity, reason
// and resource. Mounted under resourceRoutes (namespace-aware RBAC).

// cockpitRequest holds validated query parameters for the cockpit endpoint.
type cockpitRequest struct {
	WindowMinutes int // 1–10080 (7 days), default 1440 (24 hours)
	MaxGroups     int // 1–200, default 50
	PageLimit     int // number of events to fetch from gateway, capped at 1000
}

func parseCockpitRequest(c *gin.Context) (cockpitRequest, bool) {
	var req cockpitRequest
	// window_minutes
	raw := strings.TrimSpace(c.Query("window_minutes"))
	if raw == "" {
		req.WindowMinutes = 1440
	} else {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < eventcockpit.MinWindowMinutes || parsed > eventcockpit.MaxWindowMinutes {
			writeError(c, http.StatusBadRequest, "INVALID_WINDOW", "window_minutes must be between 1 and 10080")
			return req, false
		}
		req.WindowMinutes = parsed
	}
	// max_groups
	raw = strings.TrimSpace(c.Query("max_groups"))
	if raw == "" {
		req.MaxGroups = 50
	} else {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < eventcockpit.MinGroups || parsed > eventcockpit.MaxGroups {
			writeError(c, http.StatusBadRequest, "INVALID_GROUPS", "max_groups must be between 1 and 200")
			return req, false
		}
		req.MaxGroups = parsed
	}
	// page_limit (events fetched per namespace; capped at 1000)
	raw = strings.TrimSpace(c.Query("page_limit"))
	if raw == "" {
		req.PageLimit = 500
	} else {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1000 {
			writeError(c, http.StatusBadRequest, "INVALID_LIMIT", "page_limit must be between 1 and 1000")
			return req, false
		}
		req.PageLimit = parsed
	}
	return req, true
}

// cockpit fetches events across all authorized namespaces and aggregates
// them using the pure eventcockpit.Aggregate function. It follows the same
// authorization pattern as kubernetesHandler.events (namespace-aware lists).
func (h kubernetesHandler) eventCockpit(c *gin.Context) {
	req, ok := parseCockpitRequest(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	clusterID := currentClusterID(c)
	ctx := c.Request.Context()
	window := time.Duration(req.WindowMinutes) * time.Minute

	var allEvents []eventcockpit.EventInput
	if scope.AllNamespaces {
		// All-namespaces: fetch a single cluster-wide page.
		events, err := h.fetchEventsPage(ctx, clusterID, "", req.PageLimit)
		if err != nil {
			writeError(c, http.StatusServiceUnavailable, "EVENTS_UNAVAILABLE", "failed to fetch events from cluster")
			return
		}
		allEvents = append(allEvents, events...)
	} else if len(scope.NamespaceGrants) == 0 {
		// No namespace grants: empty result.
	} else {
		// Per-namespace iteration (bounded by grant count).
		for _, ns := range scope.NamespaceGrants {
			events, err := h.fetchEventsPage(ctx, clusterID, ns, req.PageLimit)
			if err != nil {
				continue // skip failed namespaces; aggregate what we have
			}
			allEvents = append(allEvents, events...)
		}
	}

	summary := eventcockpit.Aggregate(allEvents, window, time.Now().UTC(), req.MaxGroups)
	c.JSON(http.StatusOK, summary)
}

// fetchEventsPage retrieves events for one namespace (or empty for
// cluster-wide) and converts them to the pure eventcockpit.EventInput.
func (h kubernetesHandler) fetchEventsPage(ctx context.Context, clusterID int64, namespace string, pageLimit int) ([]eventcockpit.EventInput, error) {
	response, err := h.service.Events(ctx, clusterID, namespace, apiquery.ListQuery{Page: 1, Limit: pageLimit})
	if err != nil {
		return nil, err
	}
	out := make([]eventcockpit.EventInput, 0, len(response.Items))
	for _, ev := range response.Items {
		firstSeen := parseEventTime(ev.FirstTimestamp)
		lastSeen := parseEventTime(ev.LastTimestamp)
		if firstSeen.IsZero() && lastSeen.IsZero() {
			// No timestamps at all: skip the event (nothing to place in a window).
			continue
		}
		if firstSeen.IsZero() {
			firstSeen = lastSeen
		}
		if lastSeen.IsZero() {
			lastSeen = firstSeen
		}
		out = append(out, eventcockpit.EventInput{
			ID:        ev.Metadata.UID,
			Severity:  ev.Type,
			Reason:    ev.Reason,
			Message:   ev.Message,
			Count:     ev.Count,
			FirstSeen: firstSeen,
			LastSeen:  lastSeen,
			Namespace: ev.InvolvedObject.Namespace,
			Kind:      ev.InvolvedObject.Kind,
			Name:      ev.InvolvedObject.Name,
			UID:       ev.InvolvedObject.UID,
		})
	}
	return out, nil
}

// parseEventTime handles the time.RFC3339 or partial time formats used in
// Kubernetes Event timestamps. Returns zero time on failure.
func parseEventTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	// K8s uses RFC3339; try a relaxed format as fallback.
	if t, err := time.Parse("2006-01-02T15:04:05Z", raw); err == nil {
		return t
	}
	return time.Time{}
}
