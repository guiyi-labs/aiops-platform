package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/alertoverview"
	"k8s-aiops.local/backend/internal/correlation"
)

type alertOverviewRequest struct {
	WindowMinutes int // 1–10080, default 1440 (24h)
	MaxGroups     int // 1–200, default 50
	Limit         int // instances+rules fetch limit, 1–200, default 100
}

func parseAlertOverviewRequest(c *gin.Context) (alertOverviewRequest, bool) {
	var req alertOverviewRequest
	// window_minutes
	raw := strings.TrimSpace(c.Query("window_minutes"))
	if raw == "" {
		req.WindowMinutes = 1440
	} else {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < alertoverview.MinWindowMinutes || parsed > alertoverview.MaxWindowMinutes {
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
		if err != nil || parsed < alertoverview.MinGroups || parsed > alertoverview.MaxGroups {
			writeError(c, http.StatusBadRequest, "INVALID_GROUPS", "max_groups must be between 1 and 200")
			return req, false
		}
		req.MaxGroups = parsed
	}
	// limit (rules + instances fetch cap, 1–200, default 100)
	raw = strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		req.Limit = 100
	} else {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeError(c, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 200")
			return req, false
		}
		req.Limit = parsed
	}
	return req, true
}

// overview aggregates alert instances by rule with correlation-case linkage.
// Uses both alert.Service (instances + rules) and an optional
// correlation.Service (active cases for linkage).  When correlation is nil
// the response omits related_case_ids (the endpoint still functions).
func (h alertHandler) overview(c *gin.Context) {
	req, ok := parseAlertOverviewRequest(c)
	if !ok {
		return
	}
	cid, ok := clusterID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	window := time.Duration(req.WindowMinutes) * time.Minute

	// Fetch rules and instances (bounded).
	ruleList, err := h.service.ListRules(ctx, alert.RuleListFilter{ClusterID: cid, Limit: req.Limit})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "RULES_UNAVAILABLE", "failed to list alert rules")
		return
	}
	instanceList, err := h.service.ListInstances(ctx, alert.InstanceListFilter{ClusterID: cid, Limit: req.Limit})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INSTANCES_UNAVAILABLE", "failed to list alert instances")
		return
	}

	// Convert rules → overview.RuleRef.
	rules := make([]alertoverview.RuleRef, 0, len(ruleList))
	for _, r := range ruleList {
		rules = append(rules, alertoverview.RuleRef{
			ID:           r.ID,
			DisplayName:  r.DisplayName,
			ResourceKind: r.ResourceKind,
			ResourceName: r.ResourceName,
			MetricName:   r.MetricName,
		})
	}

	// Convert instances → overview.InstanceRef.
	instances := make([]alertoverview.InstanceRef, 0, len(instanceList))
	for _, inst := range instanceList {
		instances = append(instances, alertoverview.InstanceRef{
			RuleID:       inst.RuleID,
			State:        inst.State,
			FirstFiredAt: inst.FirstFiredAt,
			LastFiredAt:  inst.LastFiredAt,
			ResolvedAt:   inst.ResolvedAt,
		})
	}

	// Fetch active correlation cases for linkage (best-effort, optional).
	var cases []alertoverview.CaseRef
	if h.correlation != nil {
		caseResp, err := h.correlation.ListCases(ctx, correlation.CaseFilter{
			ClusterID: cid,
			Status:    correlation.CaseStatusActive,
			Limit:     100,
		})
		if err == nil {
			for _, cs := range caseResp.Items {
				var resources []alertoverview.ResourceRef
				resources = append(resources, alertoverview.ResourceRef{
					Kind:      cs.PrimaryResource.Kind,
					Namespace: cs.PrimaryResource.Namespace,
					Name:      cs.PrimaryResource.Name,
					UID:       cs.PrimaryResource.UID,
				})
				cases = append(cases, alertoverview.CaseRef{
					ID:         cs.ID,
					Status:     string(cs.Status),
					ReasonCode: cs.RuleID,
					Resources:  resources,
				})
			}
		}
	}

	summary := alertoverview.Aggregate(rules, instances, cases, window, time.Now().UTC(), req.MaxGroups)
	c.JSON(http.StatusOK, summary)
}
