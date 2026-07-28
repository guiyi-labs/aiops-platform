package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
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
	response, err := h.service.PodMetrics(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.Deployments(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.StatefulSets(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.DaemonSets(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.ReplicaSets(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.Jobs(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.CronJobs(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.HorizontalPodAutoscalers(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.ResourceQuotas(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.LimitRanges(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.Secrets(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.Services(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.Ingresses(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.EndpointSlices(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.PersistentVolumeClaims(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.ConfigMaps(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.Pods(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	response, err := h.service.Events(c.Request.Context(), currentClusterID(c), c.Query("namespace"), query)
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
	case errors.Is(err, k8sgateway.ErrMetricsAPIUnavailable):
		writeError(c, http.StatusFailedDependency, "METRICS_API_UNAVAILABLE", "Kubernetes Metrics API is not installed or not available")
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
