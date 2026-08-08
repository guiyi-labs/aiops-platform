package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/requestctx"
)

type kubernetesHandler struct{ service *k8sgateway.Service }

func withClusterContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := clusterID(c)
		if !ok {
			return
		}
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		metadata.ClusterID = id
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	}
}

// apiResources handles GET /api/v1/clusters/:cluster_id/api-resources.
//
// It returns the union of a fixed operator-curated GVR whitelist (always
// present, even when discovery fails) and the dynamically discovered
// resources on the cluster (CRDs and other installed API resources). The
// response is read-only GVR metadata — no resource instances are returned.
//
// This is the M47 preview of M49's full CRD browsing. Authorization is
// cluster-scoped only (no namespace dimension); the existing
// requireClusterAccess middleware gates access. 404 > 403 anti-leakage is
// preserved by the middleware.
func (h kubernetesHandler) apiResources(c *gin.Context) {
	resources, err := h.service.APIResources(c.Request.Context(), currentClusterID(c))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "DISCOVERY_FAILED", "failed to enumerate api resources")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": resources})
}

func (h kubernetesHandler) namespaces(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	response, err := h.service.Namespaces(c.Request.Context(), currentClusterID(c), query)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) nodes(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	response, err := h.service.Nodes(c.Request.Context(), currentClusterID(c), query)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) nodeMetrics(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	response, err := h.service.NodeMetrics(c.Request.Context(), currentClusterID(c), query)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) podMetrics(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.PodMetric], error) {
		return h.service.PodMetrics(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) node(c *gin.Context) {
	item, err := h.service.Node(c.Request.Context(), currentClusterID(c), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) deployments(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.Deployment], error) {
		return h.service.Deployments(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) deployment(c *gin.Context) {
	item, err := h.service.Deployment(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) statefulSets(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.StatefulSet], error) {
		return h.service.StatefulSets(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) statefulSet(c *gin.Context) {
	item, err := h.service.StatefulSet(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) daemonSets(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.DaemonSet], error) {
		return h.service.DaemonSets(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) daemonSet(c *gin.Context) {
	item, err := h.service.DaemonSet(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) replicaSets(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.ReplicaSet], error) {
		return h.service.ReplicaSets(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) replicaSet(c *gin.Context) {
	item, err := h.service.ReplicaSet(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) jobs(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.Job], error) {
		return h.service.Jobs(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) job(c *gin.Context) {
	item, err := h.service.Job(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) cronJobs(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.CronJob], error) {
		return h.service.CronJobs(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) cronJob(c *gin.Context) {
	item, err := h.service.CronJob(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) horizontalPodAutoscalers(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.HorizontalPodAutoscaler], error) {
		return h.service.HorizontalPodAutoscalers(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) horizontalPodAutoscaler(c *gin.Context) {
	item, err := h.service.HorizontalPodAutoscaler(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) resourceQuotas(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.ResourceQuota], error) {
		return h.service.ResourceQuotas(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) resourceQuota(c *gin.Context) {
	item, err := h.service.ResourceQuota(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) limitRanges(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.LimitRange], error) {
		return h.service.LimitRanges(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) limitRange(c *gin.Context) {
	item, err := h.service.LimitRange(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) secrets(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.Secret], error) {
		return h.service.Secrets(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) secret(c *gin.Context) {
	item, err := h.service.Secret(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) services(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.ServiceResource], error) {
		return h.service.Services(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) serviceDetail(c *gin.Context) {
	item, err := h.service.GetService(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) ingresses(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.Ingress], error) {
		return h.service.Ingresses(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) ingress(c *gin.Context) {
	item, err := h.service.Ingress(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) endpointSlices(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.EndpointSlice], error) {
		return h.service.EndpointSlices(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) persistentVolumeClaims(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.PersistentVolumeClaim], error) {
		return h.service.PersistentVolumeClaims(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) persistentVolumeClaim(c *gin.Context) {
	item, err := h.service.PersistentVolumeClaim(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) storageClasses(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	response, err := h.service.StorageClasses(c.Request.Context(), currentClusterID(c), query)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) storageClass(c *gin.Context) {
	item, err := h.service.StorageClass(c.Request.Context(), currentClusterID(c), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) configMaps(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.ConfigMap], error) {
		return h.service.ConfigMaps(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) configMap(c *gin.Context) {
	item, err := h.service.ConfigMap(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) pods(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.Pod], error) {
		return h.service.Pods(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) pod(c *gin.Context) {
	item, err := h.service.Pod(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) events(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.Event], error) {
		return h.service.Events(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) logs(c *gin.Context) {
	previous, err := strconv.ParseBool(defaultString(c.Query("previous"), "false"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "previous must be true or false")
		return
	}
	tailLines, err := strconv.Atoi(defaultString(c.Query("tail_lines"), "200"))
	if err != nil || tailLines < 1 || tailLines > 2000 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "tail_lines must be between 1 and 2000")
		return
	}
	logs, err := h.service.Logs(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"), c.Query("container"), previous, tailLines)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, gin.H{"logs": logs, "previous": previous})
	}
}

func (h kubernetesHandler) containers(c *gin.Context) {
	containers, err := h.service.Containers(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, gin.H{"items": containers, "total": len(containers), "remaining": 0})
	}
}

func (h kubernetesHandler) logsSince(c *gin.Context) {
	previous, err := strconv.ParseBool(defaultString(c.Query("previous"), "false"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "previous must be true or false")
		return
	}
	tailLines, err := strconv.Atoi(defaultString(c.Query("tail_lines"), "200"))
	if err != nil || tailLines < 1 || tailLines > 2000 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "tail_lines must be between 1 and 2000")
		return
	}
	sinceSeconds, _ := strconv.Atoi(c.Query("since_seconds"))
	if sinceSeconds < 0 || sinceSeconds > 86400 {
		sinceSeconds = 0
	}
	sinceTime := c.Query("since_time")
	if sinceSeconds > 0 && sinceTime != "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "provide either since_seconds or since_time, not both")
		return
	}
	log, err := h.service.LogsSince(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"), c.Query("container"), previous, tailLines, sinceSeconds, sinceTime)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, log)
	}
}

func (h kubernetesHandler) allContainerLogs(c *gin.Context) {
	previous, err := strconv.ParseBool(defaultString(c.Query("previous"), "false"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "previous must be true or false")
		return
	}
	tailLines, err := strconv.Atoi(defaultString(c.Query("tail_lines"), "200"))
	if err != nil || tailLines < 1 || tailLines > 2000 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "tail_lines must be between 1 and 2000")
		return
	}
	sinceSeconds, _ := strconv.Atoi(c.Query("since_seconds"))
	if sinceSeconds < 0 || sinceSeconds > 86400 {
		sinceSeconds = 0
	}
	response, err := h.service.AllContainerLogs(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"), previous, tailLines, sinceSeconds)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) persistentVolumes(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	response, err := h.service.PersistentVolumes(c.Request.Context(), currentClusterID(c), query)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) persistentVolume(c *gin.Context) {
	item, err := h.service.PersistentVolume(c.Request.Context(), currentClusterID(c), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) podDisruptionBudgets(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
		return h.service.PodDisruptionBudgets(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) podDisruptionBudget(c *gin.Context) {
	item, err := h.service.PodDisruptionBudget(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) networkPolicies(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.NetworkPolicy], error) {
		return h.service.NetworkPolicies(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) networkPolicy(c *gin.Context) {
	item, err := h.service.NetworkPolicy(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) serviceAccounts(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.ServiceAccount], error) {
		return h.service.ServiceAccounts(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) serviceAccount(c *gin.Context) {
	item, err := h.service.ServiceAccount(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) roles(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.Role], error) {
		return h.service.Roles(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) role(c *gin.Context) {
	item, err := h.service.Role(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) clusterRoles(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	response, err := h.service.ClusterRoles(c.Request.Context(), currentClusterID(c), query)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) clusterRole(c *gin.Context) {
	item, err := h.service.ClusterRole(c.Request.Context(), currentClusterID(c), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) roleBindings(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := c.Query("namespace")
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.RoleBinding], error) {
		return h.service.RoleBindings(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) roleBinding(c *gin.Context) {
	item, err := h.service.RoleBinding(c.Request.Context(), currentClusterID(c), c.Param("namespace"), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) clusterRoleBindings(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	response, err := h.service.ClusterRoleBindings(c.Request.Context(), currentClusterID(c), query)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) clusterRoleBinding(c *gin.Context) {
	item, err := h.service.ClusterRoleBinding(c.Request.Context(), currentClusterID(c), c.Param("name"))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) manifest(c *gin.Context) {
	kind := c.Param("kind")
	namespace := c.Param("namespace")
	name := c.Param("name")
	manifest, err := h.service.Manifest(c.Request.Context(), currentClusterID(c), kind, namespace, name)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, manifest)
	}
}

// customResources handles GET
// /api/v1/clusters/:cluster_id/custom-resources/:group/:version/:resource.
//
// Lists instances of a whitelisted CRD (ADR 0064). Namespaced CRDs fan out
// across the caller's authorized namespace scope (M35) via
// authorizedNamespaceLists and honor the optional ?workspace_id visibility
// filter (M47, applied by the middleware chain). Cluster-scoped CRDs are
// listed cluster-wide (namespace is ignored). Non-whitelisted GVRs return 404
// (anti-leakage). The response items are full manifests with sensitive fields
// redacted via redactManifest (M22 redaction reused). Read-only: no write path
// exists (ADR 0064 §2).
func (h kubernetesHandler) customResources(c *gin.Context) {
	group := c.Param("group")
	version := c.Param("version")
	resource := c.Param("resource")
	if group == "" || version == "" || resource == "" {
		writeError(c, http.StatusBadRequest, "INVALID_PATH", "group, version and resource are required path parameters")
		return
	}
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	namespaced, browsable := h.service.IsCustomResourceBrowsable(group, version, resource)
	if !browsable {
		// Non-whitelisted GVR is indistinguishable from a missing resource —
		// anti-leakage (ADR 0064 §4).
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Kubernetes resource does not exist")
		return
	}
	if !namespaced {
		// Cluster-scoped CRD: namespace dimension is irrelevant; call once.
		response, err := h.service.CustomResources(c.Request.Context(), currentClusterID(c), group, version, resource, "", query)
		if !h.writeServiceError(c, err) {
			c.JSON(http.StatusOK, response)
		}
		return
	}
	// Namespaced CRD: fan out across the caller's resolved authorization
	// scope. The ?namespace= query is honored by authorizedNamespaceLists;
	// the ?workspace_id filter was already applied by the middleware chain.
	scope := ResolvedNamespaceScope(c)
	requestedNamespace := strings.TrimSpace(c.Query("namespace"))
	response, err := authorizedNamespaceLists(c, scope, requestedNamespace, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[map[string]interface{}], error) {
		return h.service.CustomResources(ctx, clusterID, group, version, resource, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

// customResource handles GET
// /api/v1/clusters/:cluster_id/custom-resources/:group/:version/:resource/:name.
//
// Returns a single redacted CRD instance by name. For namespaced CRDs the
// ?namespace= query parameter is required (and is authz-checked by the
// requireNamespaceQueryAccess middleware in the route group). For
// cluster-scoped CRDs the namespace query is ignored. Non-whitelisted GVRs
// return 404 (anti-leakage). Read-only (ADR 0064 §2).
func (h kubernetesHandler) customResource(c *gin.Context) {
	group := c.Param("group")
	version := c.Param("version")
	resource := c.Param("resource")
	name := c.Param("name")
	if group == "" || version == "" || resource == "" || name == "" {
		writeError(c, http.StatusBadRequest, "INVALID_PATH", "group, version, resource and name are required path parameters")
		return
	}
	namespaced, browsable := h.service.IsCustomResourceBrowsable(group, version, resource)
	if !browsable {
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Kubernetes resource does not exist")
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	if namespaced && namespace == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "namespace query parameter is required for namespaced custom resources")
		return
	}
	item, err := h.service.CustomResource(c.Request.Context(), currentClusterID(c), group, version, resource, namespace, name)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) veleroCapability(c *gin.Context) {
	capability, err := h.service.VeleroCapability(c.Request.Context(), currentClusterID(c))
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, capability)
	}
}

func (h kubernetesHandler) backups(c *gin.Context) {
	query, ok := parseKubernetesListQuery(c)
	if !ok {
		return
	}
	scope := ResolvedNamespaceScope(c)
	ns := strings.TrimSpace(c.Query("namespace"))
	response, err := authorizedNamespaceLists(c, scope, ns, func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[k8sgateway.VeleroBackup], error) {
		return h.service.Backups(ctx, clusterID, namespace, query)
	})
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

func (h kubernetesHandler) backup(c *gin.Context) {
	namespace := strings.TrimSpace(c.Param("namespace"))
	name := strings.TrimSpace(c.Param("name"))
	item, err := h.service.Backup(c.Request.Context(), currentClusterID(c), namespace, name)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, item)
	}
}

func (h kubernetesHandler) writeServiceError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, cluster.ErrNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	case errors.Is(err, cluster.ErrDisabled):
		writeError(c, http.StatusConflict, "CLUSTER_DISABLED", "cluster must be enabled before querying resources")
	case errors.Is(err, k8sgateway.ErrResourceNotFound):
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Kubernetes resource does not exist")
	case errors.Is(err, k8sgateway.ErrCustomResourceNotWhitelisted):
		// Non-whitelisted CRD is indistinguishable from a missing resource —
		// anti-leakage (ADR 0064 §4). Same code/message as ErrResourceNotFound.
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Kubernetes resource does not exist")
	case errors.Is(err, k8sgateway.ErrMetricsAPIUnavailable):
		writeError(c, http.StatusFailedDependency, "METRICS_API_UNAVAILABLE", "Kubernetes Metrics API is not installed or not available")
	case errors.Is(err, k8sgateway.ErrVeleroUnavailable):
		writeError(c, http.StatusServiceUnavailable, "VELERO_UNAVAILABLE", "Velero API is not installed on this cluster")
	default:
		writeError(c, http.StatusBadGateway, "KUBERNETES_API_ERROR", "unable to query Kubernetes API")
	}
	return true
}

func currentClusterID(c *gin.Context) int64 {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	return metadata.ClusterID
}
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func parseKubernetesListQuery(c *gin.Context) (apiquery.ListQuery, bool) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return apiquery.ListQuery{}, false
	}
	return query, true
}

// authorizedNamespaceLists dispatches a namespaced resource list call through
// the user's resolved authorization scope. Behavior:
//
//  1. If scope.AllNamespaces is true (SystemAdmin or cluster-level grant):
//     call callFn(requestedNamespace) as the raw implementation would.
//  2. If requestedNamespace is a single specific namespace: middleware already
//     validated the user is authorized, so pass it through.
//  3. Otherwise (no namespace requested, no cluster-level grant, at least one
//     NamespaceGrant exists): call callFn once per authorized namespace and
//     merge Items while preserving the first error, keeping name-filtered
//     counts truthful.
//  4. Empty NamespaceGrants + no cluster-level grant + empty requested ns:
//     return an empty collection without hitting the API server.
//
// T is the list item type. The caller supplies callFn with the same service
// method signature used today.
func authorizedNamespaceLists[T any](
	c *gin.Context,
	scope authz.ClusterScope,
	requestedNamespace string,
	callFn func(ctx context.Context, clusterID int64, namespace string) (apiquery.ListResponse[T], error),
) (apiquery.ListResponse[T], error) {
	clusterID := currentClusterID(c)
	ctx := c.Request.Context()
	if scope.AllNamespaces {
		return callFn(ctx, clusterID, requestedNamespace)
	}
	// If a single namespace was explicitly requested and middleware allowed it
	// through, NamespaceGrants will contain exactly that one entry.
	if len(scope.NamespaceGrants) == 1 && requestedNamespace != "" {
		return callFn(ctx, clusterID, requestedNamespace)
	}
	// No cluster grant: aggregate per-namespace lists. Empty grants → empty.
	if len(scope.NamespaceGrants) == 0 {
		return apiquery.ListResponse[T]{}, nil
	}
	// Aggregate preserving the raw item order (sorted input stable by
	// NamespaceGrant slice order). Stop on first hard error.
	var merged []T
	var total int
	for _, ns := range scope.NamespaceGrants {
		part, err := callFn(ctx, clusterID, ns)
		if err != nil {
			return apiquery.ListResponse[T]{}, err
		}
		merged = append(merged, part.Items...)
		total += part.Total
	}
	return apiquery.ListResponse[T]{Items: merged, Total: total, Remaining: 0}, nil
}
