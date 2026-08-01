package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
)

var (
	ErrResourceNotFound      = errors.New("Kubernetes resource not found")
	ErrResourceConflict      = errors.New("Kubernetes resource already exists")
	ErrMetricsAPIUnavailable = errors.New("Kubernetes Metrics API is unavailable")
	ErrVeleroUnavailable     = errors.New("Velero API is not installed")
	ErrGitOpsUnavailable     = errors.New("ArgoCD Application API is not installed")
)

type CredentialSource interface {
	Access(context.Context, int64) (cluster.Cluster, []byte, error)
}
type Gateway interface {
	Get(context.Context, int64, []byte, string, url.Values, int64) ([]byte, error)
}
type PatchGateway interface {
	Patch(context.Context, int64, []byte, string, url.Values, string, []byte, int64) ([]byte, error)
}
type CreateGateway interface {
	Create(context.Context, int64, []byte, string, url.Values, string, []byte, int64) ([]byte, error)
}

// DiscoveryProvider exposes a client-go discovery interface for a given
// cluster. It is implemented by *cluster.ClientProvider. M47 uses it for
// /api/v1/clusters/:cluster_id/api-resources (CRD discovery preview).
type DiscoveryProvider interface {
	Discovery(clusterID int64, kubeconfig []byte) (discovery.DiscoveryInterface, error)
}

type Service struct {
	credentials CredentialSource
	gateway     Gateway
	discovery   DiscoveryProvider
}

// NewService constructs a Service. discovery may be nil — when nil, the
// APIResources method returns ErrDiscoveryUnavailable so callers can degrade
// gracefully without panicking.
func NewService(credentials CredentialSource, gateway Gateway, discovery DiscoveryProvider) *Service {
	return &Service{credentials: credentials, gateway: gateway, discovery: discovery}
}

type ObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	UID               string            `json:"uid,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	OwnerReferences   []OwnerReference  `json:"ownerReferences,omitempty"`
}

type OwnerReference struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	UID        string `json:"uid"`
	Controller *bool  `json:"controller,omitempty"`
}

type Namespace struct {
	Metadata ObjectMeta `json:"metadata"`
	Status   struct {
		Phase string `json:"phase"`
	} `json:"status"`
}
type Node struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Unschedulable bool `json:"unschedulable,omitempty"`
	} `json:"spec"`
	Status struct {
		NodeInfo struct {
			KubeletVersion          string `json:"kubeletVersion"`
			OSImage                 string `json:"osImage"`
			ContainerRuntimeVersion string `json:"containerRuntimeVersion"`
		} `json:"nodeInfo"`
		Addresses []struct {
			Type    string `json:"type"`
			Address string `json:"address"`
		} `json:"addresses"`
		Conditions []struct {
			Type               string `json:"type"`
			Status             string `json:"status"`
			Reason             string `json:"reason"`
			Message            string `json:"message"`
			LastTransitionTime string `json:"lastTransitionTime"`
		} `json:"conditions"`
		Capacity    map[string]string `json:"capacity,omitempty"`
		Allocatable map[string]string `json:"allocatable,omitempty"`
	} `json:"status"`
}

type ResourceUsage struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type NodeMetric struct {
	Metadata  ObjectMeta    `json:"metadata"`
	Timestamp string        `json:"timestamp"`
	Window    string        `json:"window"`
	Usage     ResourceUsage `json:"usage"`
}

type ContainerMetric struct {
	Name  string        `json:"name"`
	Usage ResourceUsage `json:"usage"`
}

type PodMetric struct {
	Metadata   ObjectMeta        `json:"metadata"`
	Timestamp  string            `json:"timestamp"`
	Window     string            `json:"window"`
	Containers []ContainerMetric `json:"containers"`
}
type Deployment struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Replicas *int32 `json:"replicas,omitempty"`
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels,omitempty"`
		} `json:"selector"`
		Template WorkloadTemplate `json:"template"`
	} `json:"spec"`
	Status struct {
		Replicas            int32               `json:"replicas"`
		ReadyReplicas       int32               `json:"readyReplicas"`
		AvailableReplicas   int32               `json:"availableReplicas"`
		UpdatedReplicas     int32               `json:"updatedReplicas"`
		UnavailableReplicas int32               `json:"unavailableReplicas"`
		Conditions          []WorkloadCondition `json:"conditions,omitempty"`
	} `json:"status"`
}

type WorkloadContainer struct {
	Name      string               `json:"name"`
	Image     string               `json:"image"`
	Resources ResourceRequirements `json:"resources,omitempty"`
}

type ResourceRequirements struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

type WorkloadCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

type WorkloadTemplate struct {
	Spec struct {
		Containers []WorkloadContainer `json:"containers"`
	} `json:"spec"`
	Raw json.RawMessage `json:"-"`
}

func (template *WorkloadTemplate) UnmarshalJSON(data []byte) error {
	type workloadTemplateAlias WorkloadTemplate
	var decoded workloadTemplateAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*template = WorkloadTemplate(decoded)
	template.Raw = append(template.Raw[:0], data...)
	return nil
}

type StatefulSet struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Replicas            *int32 `json:"replicas,omitempty"`
		ServiceName         string `json:"serviceName"`
		PodManagementPolicy string `json:"podManagementPolicy,omitempty"`
		Selector            struct {
			MatchLabels map[string]string `json:"matchLabels,omitempty"`
		} `json:"selector"`
		Template       WorkloadTemplate `json:"template"`
		UpdateStrategy struct {
			Type string `json:"type"`
		} `json:"updateStrategy"`
	} `json:"spec"`
	Status struct {
		Replicas          int32 `json:"replicas"`
		CurrentReplicas   int32 `json:"currentReplicas"`
		ReadyReplicas     int32 `json:"readyReplicas"`
		UpdatedReplicas   int32 `json:"updatedReplicas"`
		AvailableReplicas int32 `json:"availableReplicas"`
	} `json:"status"`
}

type DaemonSet struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels,omitempty"`
		} `json:"selector"`
		Template       WorkloadTemplate `json:"template"`
		UpdateStrategy struct {
			Type string `json:"type"`
		} `json:"updateStrategy"`
	} `json:"spec"`
	Status struct {
		DesiredNumberScheduled int32 `json:"desiredNumberScheduled"`
		CurrentNumberScheduled int32 `json:"currentNumberScheduled"`
		NumberReady            int32 `json:"numberReady"`
		NumberAvailable        int32 `json:"numberAvailable"`
		UpdatedNumberScheduled int32 `json:"updatedNumberScheduled"`
		NumberUnavailable      int32 `json:"numberUnavailable"`
	} `json:"status"`
}

type ReplicaSet struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Replicas *int32 `json:"replicas,omitempty"`
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels,omitempty"`
		} `json:"selector"`
		Template WorkloadTemplate `json:"template"`
	} `json:"spec"`
	Status struct {
		Replicas          int32 `json:"replicas"`
		ReadyReplicas     int32 `json:"readyReplicas"`
		AvailableReplicas int32 `json:"availableReplicas"`
		FullyLabeled      int32 `json:"fullyLabeledReplicas"`
	} `json:"status"`
}

type JobSpec struct {
	Parallelism  *int32           `json:"parallelism,omitempty"`
	Completions  *int32           `json:"completions,omitempty"`
	BackoffLimit *int32           `json:"backoffLimit,omitempty"`
	Suspend      *bool            `json:"suspend,omitempty"`
	Template     WorkloadTemplate `json:"template"`
}

type Job struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     JobSpec    `json:"spec"`
	Status   struct {
		Active         int32               `json:"active"`
		Succeeded      int32               `json:"succeeded"`
		Failed         int32               `json:"failed"`
		StartTime      string              `json:"startTime,omitempty"`
		CompletionTime string              `json:"completionTime,omitempty"`
		Conditions     []WorkloadCondition `json:"conditions,omitempty"`
	} `json:"status"`
}

type CronJob struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Schedule                   string `json:"schedule"`
		TimeZone                   string `json:"timeZone,omitempty"`
		ConcurrencyPolicy          string `json:"concurrencyPolicy,omitempty"`
		Suspend                    *bool  `json:"suspend,omitempty"`
		SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`
		FailedJobsHistoryLimit     *int32 `json:"failedJobsHistoryLimit,omitempty"`
		JobTemplate                struct {
			Spec JobSpec `json:"spec"`
		} `json:"jobTemplate"`
	} `json:"spec"`
	Status struct {
		Active []struct {
			Kind      string `json:"kind,omitempty"`
			Namespace string `json:"namespace,omitempty"`
			Name      string `json:"name,omitempty"`
			UID       string `json:"uid,omitempty"`
		} `json:"active,omitempty"`
		LastScheduleTime   string `json:"lastScheduleTime,omitempty"`
		LastSuccessfulTime string `json:"lastSuccessfulTime,omitempty"`
	} `json:"status"`
}

type MetricTarget struct {
	Type               string `json:"type"`
	Value              string `json:"value,omitempty"`
	AverageValue       string `json:"averageValue,omitempty"`
	AverageUtilization *int32 `json:"averageUtilization,omitempty"`
}

type MetricIdentifier struct {
	Name string `json:"name"`
}

type HPAMetricSpec struct {
	Type     string `json:"type"`
	Resource *struct {
		Name   string       `json:"name"`
		Target MetricTarget `json:"target"`
	} `json:"resource,omitempty"`
	ContainerResource *struct {
		Name      string       `json:"name"`
		Container string       `json:"container"`
		Target    MetricTarget `json:"target"`
	} `json:"containerResource,omitempty"`
	Pods *struct {
		Metric MetricIdentifier `json:"metric"`
		Target MetricTarget     `json:"target"`
	} `json:"pods,omitempty"`
	Object *struct {
		DescribedObject struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Name       string `json:"name"`
		} `json:"describedObject"`
		Metric MetricIdentifier `json:"metric"`
		Target MetricTarget     `json:"target"`
	} `json:"object,omitempty"`
	External *struct {
		Metric MetricIdentifier `json:"metric"`
		Target MetricTarget     `json:"target"`
	} `json:"external,omitempty"`
}

type HorizontalPodAutoscaler struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		ScaleTargetRef struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Name       string `json:"name"`
		} `json:"scaleTargetRef"`
		MinReplicas *int32          `json:"minReplicas,omitempty"`
		MaxReplicas int32           `json:"maxReplicas"`
		Metrics     []HPAMetricSpec `json:"metrics"`
	} `json:"spec"`
	Status struct {
		CurrentReplicas int32               `json:"currentReplicas"`
		DesiredReplicas int32               `json:"desiredReplicas"`
		LastScaleTime   string              `json:"lastScaleTime,omitempty"`
		Conditions      []WorkloadCondition `json:"conditions"`
	} `json:"status"`
}

type ResourceQuota struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Hard map[string]string `json:"hard,omitempty"`
	} `json:"spec"`
	Status struct {
		Hard map[string]string `json:"hard,omitempty"`
		Used map[string]string `json:"used,omitempty"`
	} `json:"status"`
}

type LimitRangeItem struct {
	Type                 string            `json:"type"`
	Max                  map[string]string `json:"max,omitempty"`
	Min                  map[string]string `json:"min,omitempty"`
	Default              map[string]string `json:"default,omitempty"`
	DefaultRequest       map[string]string `json:"defaultRequest,omitempty"`
	MaxLimitRequestRatio map[string]string `json:"maxLimitRequestRatio,omitempty"`
}

type LimitRange struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Limits []LimitRangeItem `json:"limits"`
	} `json:"spec"`
}

type Secret struct {
	Metadata  ObjectMeta `json:"metadata"`
	Immutable *bool      `json:"immutable,omitempty"`
	Type      string     `json:"type"`
	DataKeys  []string   `json:"dataKeys"`
}

type rawSecret struct {
	Metadata  ObjectMeta        `json:"metadata"`
	Immutable *bool             `json:"immutable,omitempty"`
	Type      string            `json:"type"`
	Data      map[string]string `json:"data,omitempty"`
}

func (s *Service) Deployment(ctx context.Context, clusterID int64, namespace, name string) (Deployment, error) {
	var item Deployment
	err := s.getJSON(ctx, clusterID, "/apis/apps/v1/namespaces/"+url.PathEscape(namespace)+"/deployments/"+url.PathEscape(name), nil, &item)
	return item, err
}

type RolloutRevision struct {
	Revision          int32    `json:"revision"`
	ReplicaSetName    string   `json:"replicaset_name"`
	UID               string   `json:"uid"`
	ResourceVersion   string   `json:"resource_version"`
	CreatedAt         string   `json:"created_at"`
	Replicas          int32    `json:"replicas"`
	ReadyReplicas     int32    `json:"ready_replicas"`
	AvailableReplicas int32    `json:"available_replicas"`
	Images            []string `json:"images"`
	Current           bool     `json:"current"`
}

type RolloutHistory struct {
	Deployment      string            `json:"deployment"`
	Namespace       string            `json:"namespace"`
	CurrentRevision int32             `json:"current_revision"`
	Revisions       []RolloutRevision `json:"revisions"`
}

type RolloutStatus struct {
	Deployment          string              `json:"deployment"`
	Namespace           string              `json:"namespace"`
	CurrentRevision     int32               `json:"current_revision"`
	DesiredReplicas     int32               `json:"desired_replicas"`
	UpdatedReplicas     int32               `json:"updated_replicas"`
	ReadyReplicas       int32               `json:"ready_replicas"`
	AvailableReplicas   int32               `json:"available_replicas"`
	UnavailableReplicas int32               `json:"unavailable_replicas"`
	Phase               string              `json:"phase"`
	Reason              string              `json:"reason,omitempty"`
	Message             string              `json:"message,omitempty"`
	Conditions          []WorkloadCondition `json:"conditions,omitempty"`
}

const deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"

func (s *Service) ReplicaSetsByOwner(ctx context.Context, clusterID int64, namespace, ownerUID string) ([]ReplicaSet, error) {
	path := namespacedListPath("/apis/apps/v1", namespace, "replicasets")
	var envelope listEnvelope[ReplicaSet]
	if err := s.getJSON(ctx, clusterID, path, nil, &envelope); err != nil {
		return nil, err
	}
	owned := make([]ReplicaSet, 0, len(envelope.Items))
	for _, item := range envelope.Items {
		for _, owner := range item.Metadata.OwnerReferences {
			if owner.UID == ownerUID && owner.Kind == "Deployment" {
				owned = append(owned, item)
				break
			}
		}
	}
	return owned, nil
}

func (s *Service) RolloutHistory(ctx context.Context, clusterID int64, namespace, name string) (RolloutHistory, error) {
	deployment, err := s.Deployment(ctx, clusterID, namespace, name)
	if err != nil {
		return RolloutHistory{}, err
	}
	if deployment.Metadata.UID == "" {
		return RolloutHistory{}, ErrResourceNotFound
	}
	currentRevision := parseRevision(deployment.Metadata.Annotations[deploymentRevisionAnnotation])
	replicaSets, err := s.ReplicaSetsByOwner(ctx, clusterID, namespace, deployment.Metadata.UID)
	if err != nil {
		return RolloutHistory{}, err
	}
	revisions := make([]RolloutRevision, 0, len(replicaSets))
	for _, rs := range replicaSets {
		revision := parseRevision(rs.Metadata.Annotations[deploymentRevisionAnnotation])
		if revision == 0 {
			continue
		}
		images := make([]string, 0, len(rs.Spec.Template.Spec.Containers))
		for _, container := range rs.Spec.Template.Spec.Containers {
			images = append(images, container.Image)
		}
		revisions = append(revisions, RolloutRevision{
			Revision:          revision,
			ReplicaSetName:    rs.Metadata.Name,
			UID:               rs.Metadata.UID,
			ResourceVersion:   rs.Metadata.ResourceVersion,
			CreatedAt:         rs.Metadata.CreationTimestamp,
			Replicas:          rs.Status.Replicas,
			ReadyReplicas:     rs.Status.ReadyReplicas,
			AvailableReplicas: rs.Status.AvailableReplicas,
			Images:            images,
			Current:           revision == currentRevision,
		})
	}
	sort.SliceStable(revisions, func(i, j int) bool { return revisions[i].Revision < revisions[j].Revision })
	return RolloutHistory{Deployment: name, Namespace: namespace, CurrentRevision: currentRevision, Revisions: revisions}, nil
}

func (s *Service) RolloutStatus(ctx context.Context, clusterID int64, namespace, name string) (RolloutStatus, error) {
	deployment, err := s.Deployment(ctx, clusterID, namespace, name)
	if err != nil {
		return RolloutStatus{}, err
	}
	if deployment.Metadata.UID == "" {
		return RolloutStatus{}, ErrResourceNotFound
	}
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	currentRevision := parseRevision(deployment.Metadata.Annotations[deploymentRevisionAnnotation])
	status := RolloutStatus{
		Deployment:          name,
		Namespace:           namespace,
		CurrentRevision:     currentRevision,
		DesiredReplicas:     desired,
		UpdatedReplicas:     deployment.Status.UpdatedReplicas,
		ReadyReplicas:       deployment.Status.ReadyReplicas,
		AvailableReplicas:   deployment.Status.AvailableReplicas,
		UnavailableReplicas: deployment.Status.UnavailableReplicas,
		Conditions:          deployment.Status.Conditions,
	}
	status.Phase, status.Reason, status.Message = deriveRolloutPhase(deployment.Status.Conditions, deployment.Status.UpdatedReplicas, deployment.Status.UnavailableReplicas, deployment.Status.ReadyReplicas, deployment.Status.AvailableReplicas, desired)
	if status.Conditions == nil {
		status.Conditions = []WorkloadCondition{}
	}
	return status, nil
}

func deriveRolloutPhase(conditions []WorkloadCondition, updated, unavailable, ready, available, desired int32) (phase, reason, message string) {
	for _, condition := range conditions {
		if condition.Type == "Progressing" && condition.Status == "False" && condition.Reason != "NewReplicaSetCreated" {
			return "failed", condition.Reason, condition.Message
		}
	}
	if updated < desired || unavailable > 0 {
		for _, condition := range conditions {
			if condition.Type == "Progressing" {
				return "progressing", condition.Reason, condition.Message
			}
		}
		return "progressing", "", ""
	}
	if ready >= desired && available >= desired {
		return "complete", "", ""
	}
	return "progressing", "", ""
}

func parseRevision(value string) int32 {
	n, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

func (s *Service) Node(ctx context.Context, clusterID int64, name string) (Node, error) {
	var item Node
	err := s.getJSON(ctx, clusterID, "/api/v1/nodes/"+url.PathEscape(name), nil, &item)
	return item, err
}

func (s *Service) PatchDeployment(ctx context.Context, clusterID int64, namespace, name string, patch []byte, dryRun bool) (Deployment, error) {
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return Deployment{}, err
	}
	gateway, ok := s.gateway.(PatchGateway)
	if !ok {
		return Deployment{}, errors.New("Kubernetes mutation gateway is unavailable")
	}
	query := url.Values{}
	if dryRun {
		query.Set("dryRun", "All")
	}
	body, err := gateway.Patch(ctx, clusterID, kubeconfig, "/apis/apps/v1/namespaces/"+url.PathEscape(namespace)+"/deployments/"+url.PathEscape(name), query, "application/strategic-merge-patch+json", patch, 10<<20)
	if err != nil {
		return Deployment{}, mapGatewayError(err)
	}
	var item Deployment
	if err := json.Unmarshal(body, &item); err != nil {
		return Deployment{}, fmt.Errorf("decode Kubernetes API response: %w", err)
	}
	return item, nil
}

func (s *Service) PatchCronJob(ctx context.Context, clusterID int64, namespace, name string, patch []byte, dryRun bool) (CronJob, error) {
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return CronJob{}, err
	}
	gateway, ok := s.gateway.(PatchGateway)
	if !ok {
		return CronJob{}, errors.New("Kubernetes mutation gateway is unavailable")
	}
	query := url.Values{}
	if dryRun {
		query.Set("dryRun", "All")
	}
	body, err := gateway.Patch(ctx, clusterID, kubeconfig, namespacedDetailPath("/apis/batch/v1", namespace, "cronjobs", name), query, "application/strategic-merge-patch+json", patch, 10<<20)
	if err != nil {
		return CronJob{}, mapGatewayError(err)
	}
	var item CronJob
	if err := json.Unmarshal(body, &item); err != nil {
		return CronJob{}, fmt.Errorf("decode Kubernetes API response: %w", err)
	}
	return item, nil
}

// PatchNode patches a Node resource (used for cordon/uncordon via
// spec.unschedulable). It follows the same PatchGateway pattern as
// PatchDeployment/PatchCronJob.
func (s *Service) PatchNode(ctx context.Context, clusterID int64, name string, patch []byte, dryRun bool) (Node, error) {
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return Node{}, err
	}
	gateway, ok := s.gateway.(PatchGateway)
	if !ok {
		return Node{}, errors.New("Kubernetes mutation gateway is unavailable")
	}
	query := url.Values{}
	if dryRun {
		query.Set("dryRun", "All")
	}
	body, err := gateway.Patch(ctx, clusterID, kubeconfig, "/api/v1/nodes/"+url.PathEscape(name), query, "application/strategic-merge-patch+json", patch, 10<<20)
	if err != nil {
		return Node{}, mapGatewayError(err)
	}
	var item Node
	if err := json.Unmarshal(body, &item); err != nil {
		return Node{}, fmt.Errorf("decode Kubernetes API response: %w", err)
	}
	return item, nil
}

type ServiceResource struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Type         string            `json:"type"`
		ClusterIP    string            `json:"clusterIP,omitempty"`
		ExternalName string            `json:"externalName,omitempty"`
		Selector     map[string]string `json:"selector,omitempty"`
		Ports        []struct {
			Name       string `json:"name,omitempty"`
			Protocol   string `json:"protocol"`
			Port       int32  `json:"port"`
			TargetPort any    `json:"targetPort,omitempty"`
			NodePort   int32  `json:"nodePort,omitempty"`
		} `json:"ports"`
	} `json:"spec"`
}

type IngressServiceBackend struct {
	Name string `json:"name"`
	Port struct {
		Name   string `json:"name,omitempty"`
		Number int32  `json:"number,omitempty"`
	} `json:"port"`
}

type IngressBackend struct {
	Service *IngressServiceBackend `json:"service,omitempty"`
}

type Ingress struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		IngressClassName string          `json:"ingressClassName,omitempty"`
		DefaultBackend   *IngressBackend `json:"defaultBackend,omitempty"`
		Rules            []struct {
			Host string `json:"host,omitempty"`
			HTTP *struct {
				Paths []struct {
					Path     string         `json:"path,omitempty"`
					PathType string         `json:"pathType,omitempty"`
					Backend  IngressBackend `json:"backend"`
				} `json:"paths"`
			} `json:"http,omitempty"`
		} `json:"rules,omitempty"`
		TLS []struct {
			Hosts      []string `json:"hosts,omitempty"`
			SecretName string   `json:"secretName,omitempty"`
		} `json:"tls,omitempty"`
	} `json:"spec"`
	Status struct {
		LoadBalancer struct {
			Ingress []struct {
				IP       string `json:"ip,omitempty"`
				Hostname string `json:"hostname,omitempty"`
			} `json:"ingress,omitempty"`
		} `json:"loadBalancer"`
	} `json:"status"`
}

type PersistentVolumeClaim struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		AccessModes      []string `json:"accessModes,omitempty"`
		StorageClassName *string  `json:"storageClassName,omitempty"`
		VolumeMode       *string  `json:"volumeMode,omitempty"`
		VolumeName       string   `json:"volumeName,omitempty"`
		Resources        struct {
			Requests map[string]string `json:"requests,omitempty"`
		} `json:"resources"`
	} `json:"spec"`
	Status struct {
		Phase       string            `json:"phase"`
		AccessModes []string          `json:"accessModes,omitempty"`
		Capacity    map[string]string `json:"capacity,omitempty"`
	} `json:"status"`
}

type StorageClass struct {
	Metadata             ObjectMeta `json:"metadata"`
	Provisioner          string     `json:"provisioner"`
	ReclaimPolicy        string     `json:"reclaimPolicy,omitempty"`
	VolumeBindingMode    string     `json:"volumeBindingMode,omitempty"`
	AllowVolumeExpansion *bool      `json:"allowVolumeExpansion,omitempty"`
}

type PersistentVolume struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Capacity                      map[string]string `json:"capacity,omitempty"`
		AccessModes                   []string          `json:"accessModes,omitempty"`
		PersistentVolumeReclaimPolicy string            `json:"persistentVolumeReclaimPolicy,omitempty"`
		StorageClassName              string            `json:"storageClassName,omitempty"`
		VolumeMode                    string            `json:"volumeMode,omitempty"`
		ClaimRef                      *struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"claimRef,omitempty"`
	} `json:"spec"`
	Status struct {
		Phase   string `json:"phase"`
		Message string `json:"message,omitempty"`
	} `json:"status"`
}

type PodDisruptionBudget struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		MinAvailable   interface{}           `json:"minAvailable,omitempty"`
		MaxUnavailable interface{}           `json:"maxUnavailable,omitempty"`
		Selector       *metav1.LabelSelector `json:"selector,omitempty"`
	} `json:"spec"`
	Status struct {
		CurrentHealthy     int32 `json:"currentHealthy"`
		DesiredHealthy     int32 `json:"desiredHealthy"`
		DisruptionsAllowed int32 `json:"disruptionsAllowed"`
		ExpectedPods       int32 `json:"expectedPods"`
	} `json:"status"`
}

type NetworkPolicy struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		PodSelector interface{}   `json:"podSelector,omitempty"`
		PolicyTypes []string      `json:"policyTypes,omitempty"`
		Ingress     []interface{} `json:"ingress,omitempty"`
		Egress      []interface{} `json:"egress,omitempty"`
	} `json:"spec"`
}

type ServiceAccount struct {
	Metadata                     ObjectMeta `json:"metadata"`
	AutomountServiceAccountToken *bool      `json:"automountServiceAccountToken,omitempty"`
	ImagePullSecrets             []struct {
		Name string `json:"name"`
	} `json:"imagePullSecrets,omitempty"`
}

// PolicyRule is the bounded projection of a Kubernetes RBAC PolicyRule. Verbs
// and resources are kept as-is because they are already bounded enumerations
// in the Kubernetes API; nonResourceURLs is included for ClusterRole audit.
type PolicyRule struct {
	APIGroups       []string `json:"apiGroups,omitempty"`
	Resources       []string `json:"resources,omitempty"`
	ResourceNames   []string `json:"resourceNames,omitempty"`
	Verbs           []string `json:"verbs"`
	NonResourceURLs []string `json:"nonResourceURLs,omitempty"`
}

// Role is the bounded projection of a namespaced Kubernetes Role.
type Role struct {
	Metadata ObjectMeta   `json:"metadata"`
	Rules    []PolicyRule `json:"rules"`
}

// ClusterRole is the bounded projection of a cluster-scoped Kubernetes
// ClusterRole. aggregationRule is omitted on purpose: the platform does not
// edit or merge aggregation rules, only inventories them.
type ClusterRole struct {
	Metadata ObjectMeta   `json:"metadata"`
	Rules    []PolicyRule `json:"rules"`
}

// RoleRef is the bounded projection of a Kubernetes RoleRef.
type RoleRef struct {
	APIGroup string `json:"apiGroup"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}

// Subject is the bounded projection of a Kubernetes Subject. The platform
// never inventories ServiceAccount tokens, only the Subject reference.
type Subject struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	APIGroup  string `json:"apiGroup,omitempty"`
}

// RoleBinding is the bounded projection of a namespaced Kubernetes RoleBinding.
type RoleBinding struct {
	Metadata ObjectMeta `json:"metadata"`
	RoleRef  RoleRef    `json:"roleRef"`
	Subjects []Subject  `json:"subjects"`
}

// ClusterRoleBinding is the bounded projection of a cluster-scoped Kubernetes
// ClusterRoleBinding.
type ClusterRoleBinding struct {
	Metadata ObjectMeta `json:"metadata"`
	RoleRef  RoleRef    `json:"roleRef"`
	Subjects []Subject  `json:"subjects"`
}

type ConfigMap struct {
	Metadata       ObjectMeta `json:"metadata"`
	Immutable      *bool      `json:"immutable,omitempty"`
	DataKeys       []string   `json:"dataKeys"`
	BinaryDataKeys []string   `json:"binaryDataKeys"`
}

type rawConfigMap struct {
	Metadata   ObjectMeta        `json:"metadata"`
	Immutable  *bool             `json:"immutable,omitempty"`
	Data       map[string]string `json:"data,omitempty"`
	BinaryData map[string]string `json:"binaryData,omitempty"`
}

// VeleroCapability reports whether the Velero API group is installed on the
// target cluster. The platform never makes Velero a core dependency; callers
// must check Installed before listing backups.
type VeleroCapability struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
}

// VeleroBackup is the bounded, read-only projection of a Velero Backup CR.
// It carries scope (included namespaces), phase, expiry, failure details and
// timestamps — not the full Velero spec/status.
type VeleroBackup struct {
	Name                    string   `json:"name"`
	Namespace               string   `json:"namespace"`
	UID                     string   `json:"uid"`
	ResourceVersion         string   `json:"resource_version"`
	Phase                   string   `json:"phase"`
	IncludedNamespaces      []string `json:"included_namespaces,omitempty"`
	StorageLocation         string   `json:"storage_location,omitempty"`
	TTL                     string   `json:"ttl,omitempty"`
	IncludeClusterResources *bool    `json:"include_cluster_resources,omitempty"`
	SnapshotVolumes         *bool    `json:"snapshot_volumes,omitempty"`
	HasLabelSelector        bool     `json:"has_label_selector"`
	Expiration              string   `json:"expiration,omitempty"`
	StartedAt               string   `json:"started_at,omitempty"`
	CompletedAt             string   `json:"completed_at,omitempty"`
	FailureReason           string   `json:"failure_reason,omitempty"`
	Errors                  int      `json:"errors"`
	Warnings                int      `json:"warnings"`
	CreatedAt               string   `json:"created_at"`
}

// rawVeleroBackup is the raw Velero Backup CR shape used for decoding. It is
// not exposed to HTTP callers; VeleroBackup is the public projection.
type rawVeleroBackup struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		IncludedNamespaces      []string    `json:"includedNamespaces,omitempty"`
		StorageLocation         string      `json:"storageLocation,omitempty"`
		IncludeClusterResources *bool       `json:"includeClusterResources,omitempty"`
		SnapshotVolumes         *bool       `json:"snapshotVolumes,omitempty"`
		TTL                     string      `json:"ttl,omitempty"`
		LabelSelector           interface{} `json:"labelSelector,omitempty"`
	} `json:"spec"`
	Status struct {
		Phase               string `json:"phase,omitempty"`
		Expiration          string `json:"expiration,omitempty"`
		StartTimestamp      string `json:"startTimestamp,omitempty"`
		CompletionTimestamp string `json:"completionTimestamp,omitempty"`
		FailureReason       string `json:"failureReason,omitempty"`
		Errors              int    `json:"errors,omitempty"`
		Warnings            int    `json:"warnings,omitempty"`
	} `json:"status"`
}

func (raw rawVeleroBackup) project() VeleroBackup {
	return VeleroBackup{
		Name:                    raw.Metadata.Name,
		Namespace:               raw.Metadata.Namespace,
		UID:                     raw.Metadata.UID,
		ResourceVersion:         raw.Metadata.ResourceVersion,
		Phase:                   raw.Status.Phase,
		IncludedNamespaces:      raw.Spec.IncludedNamespaces,
		StorageLocation:         raw.Spec.StorageLocation,
		TTL:                     raw.Spec.TTL,
		IncludeClusterResources: raw.Spec.IncludeClusterResources,
		SnapshotVolumes:         raw.Spec.SnapshotVolumes,
		HasLabelSelector:        raw.Spec.LabelSelector != nil,
		Expiration:              raw.Status.Expiration,
		StartedAt:               raw.Status.StartTimestamp,
		CompletedAt:             raw.Status.CompletionTimestamp,
		FailureReason:           raw.Status.FailureReason,
		Errors:                  raw.Status.Errors,
		Warnings:                raw.Status.Warnings,
		CreatedAt:               raw.Metadata.CreationTimestamp,
	}
}

type EndpointTargetRef struct {
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	UID       string `json:"uid,omitempty"`
}
type EndpointAddress struct {
	IP        string             `json:"ip"`
	NodeName  string             `json:"nodeName,omitempty"`
	TargetRef *EndpointTargetRef `json:"targetRef,omitempty"`
}
type EndpointPort struct {
	Name     string `json:"name,omitempty"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
}
type EndpointSubset struct {
	Addresses         []EndpointAddress `json:"addresses,omitempty"`
	NotReadyAddresses []EndpointAddress `json:"notReadyAddresses,omitempty"`
	Ports             []EndpointPort    `json:"ports,omitempty"`
}
type Endpoints struct {
	Metadata  ObjectMeta       `json:"metadata"`
	Subsets   []EndpointSubset `json:"subsets,omitempty"`
	SourceAPI string           `json:"-"`
}
type EndpointSlice struct {
	Metadata    ObjectMeta              `json:"metadata"`
	AddressType string                  `json:"addressType"`
	ServiceName string                  `json:"serviceName,omitempty"`
	Ports       []EndpointSlicePort     `json:"ports,omitempty"`
	Endpoints   []EndpointSliceEndpoint `json:"endpoints"`
}
type EndpointSlicePort struct {
	Name     string `json:"name,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Port     int32  `json:"port,omitempty"`
}
type EndpointSliceEndpoint struct {
	Addresses  []string `json:"addresses"`
	Conditions struct {
		Ready       *bool `json:"ready,omitempty"`
		Serving     *bool `json:"serving,omitempty"`
		Terminating *bool `json:"terminating,omitempty"`
	} `json:"conditions,omitempty"`
	NodeName  *string            `json:"nodeName,omitempty"`
	TargetRef *EndpointTargetRef `json:"targetRef,omitempty"`
}
type ContainerStateDetail struct {
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	ExitCode   int32  `json:"exitCode,omitempty"`
	Signal     int32  `json:"signal,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
}
type ContainerState struct {
	Waiting    *ContainerStateDetail `json:"waiting,omitempty"`
	Terminated *ContainerStateDetail `json:"terminated,omitempty"`
}
type ContainerStatus struct {
	Name         string         `json:"name"`
	Ready        bool           `json:"ready"`
	RestartCount int32          `json:"restartCount"`
	State        ContainerState `json:"state"`
	LastState    ContainerState `json:"lastState"`
}
type PodCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}
type PodVolume struct {
	Name                  string                             `json:"name"`
	EmptyDir              *json.RawMessage                   `json:"emptyDir,omitempty"`
	PersistentVolumeClaim *PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`
}

// PersistentVolumeClaimVolumeSource is the bounded projection of a Pod volume
// reference to a PVC. Only the claim name is needed for topology derivation.
type PersistentVolumeClaimVolumeSource struct {
	ClaimName string `json:"claimName"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}
type PodContainer struct {
	Name      string               `json:"name"`
	Image     string               `json:"image"`
	Resources ResourceRequirements `json:"resources,omitempty"`
}
type Pod struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		NodeName       string         `json:"nodeName,omitempty"`
		Containers     []PodContainer `json:"containers"`
		InitContainers []PodContainer `json:"initContainers,omitempty"`
		Volumes        []PodVolume    `json:"volumes,omitempty"`
	} `json:"spec"`
	Status struct {
		Phase                 string            `json:"phase"`
		PodIP                 string            `json:"podIP,omitempty"`
		HostIP                string            `json:"hostIP,omitempty"`
		Reason                string            `json:"reason,omitempty"`
		Message               string            `json:"message,omitempty"`
		Conditions            []PodCondition    `json:"conditions,omitempty"`
		ContainerStatuses     []ContainerStatus `json:"containerStatuses,omitempty"`
		InitContainerStatuses []ContainerStatus `json:"initContainerStatuses,omitempty"`
	} `json:"status"`
}
type Event struct {
	Metadata           ObjectMeta `json:"metadata"`
	Type               string     `json:"type"`
	Reason             string     `json:"reason"`
	Message            string     `json:"message"`
	Count              int32      `json:"count"`
	Action             string     `json:"action,omitempty"`
	EventTime          string     `json:"eventTime,omitempty"`
	FirstTimestamp     string     `json:"firstTimestamp,omitempty"`
	LastTimestamp      string     `json:"lastTimestamp,omitempty"`
	ReportingComponent string     `json:"reportingComponent,omitempty"`
	ReportingInstance  string     `json:"reportingInstance,omitempty"`
	Series             struct {
		Count            int32  `json:"count,omitempty"`
		LastObservedTime string `json:"lastObservedTime,omitempty"`
	} `json:"series,omitempty"`
	InvolvedObject struct {
		Kind      string `json:"kind"`
		Namespace string `json:"namespace,omitempty"`
		Name      string `json:"name"`
		UID       string `json:"uid,omitempty"`
	} `json:"involvedObject"`
}
type listEnvelope[T any] struct {
	Items []T `json:"items"`
}

func (s *Service) Namespaces(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[Namespace], error) {
	var envelope listEnvelope[Namespace]
	if err := s.getJSON(ctx, clusterID, "/api/v1/namespaces", selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[Namespace]{}, err
	}
	items := filterAndPage(envelope.Items, query, func(item Namespace) string { return item.Metadata.Name })
	return apiquery.ListResponse[Namespace]{Items: items, Total: countNamed(envelope.Items, query.Name, func(item Namespace) string { return item.Metadata.Name }), Remaining: remaining(len(items), countNamed(envelope.Items, query.Name, func(item Namespace) string { return item.Metadata.Name }), query.Offset)}, nil
}

func (s *Service) Nodes(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[Node], error) {
	var envelope listEnvelope[Node]
	if err := s.getJSON(ctx, clusterID, "/api/v1/nodes", selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[Node]{}, err
	}
	return pageResponse(envelope.Items, query, func(item Node) string { return item.Metadata.Name }), nil
}

func (s *Service) NodeMetrics(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[NodeMetric], error) {
	var envelope listEnvelope[NodeMetric]
	if err := s.metricsJSON(ctx, clusterID, "/apis/metrics.k8s.io/v1beta1/nodes", selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[NodeMetric]{}, err
	}
	return pageResponse(envelope.Items, query, func(item NodeMetric) string { return item.Metadata.Name }), nil
}

func (s *Service) PodMetrics(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[PodMetric], error) {
	path := "/apis/metrics.k8s.io/v1beta1/pods"
	if namespace != "" {
		path = "/apis/metrics.k8s.io/v1beta1/namespaces/" + url.PathEscape(namespace) + "/pods"
	}
	var envelope listEnvelope[PodMetric]
	if err := s.metricsJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[PodMetric]{}, err
	}
	for index := range envelope.Items {
		if envelope.Items[index].Containers == nil {
			envelope.Items[index].Containers = []ContainerMetric{}
		}
	}
	return pageResponse(envelope.Items, query, func(item PodMetric) string { return item.Metadata.Name }), nil
}

func (s *Service) Deployments(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[Deployment], error) {
	path := "/apis/apps/v1/deployments"
	if namespace != "" {
		path = "/apis/apps/v1/namespaces/" + url.PathEscape(namespace) + "/deployments"
	}
	var envelope listEnvelope[Deployment]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[Deployment]{}, err
	}
	return pageResponse(envelope.Items, query, func(item Deployment) string { return item.Metadata.Name }), nil
}

func (s *Service) StatefulSets(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[StatefulSet], error) {
	path := namespacedListPath("/apis/apps/v1", namespace, "statefulsets")
	var envelope listEnvelope[StatefulSet]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[StatefulSet]{}, err
	}
	return pageResponse(envelope.Items, query, func(item StatefulSet) string { return item.Metadata.Name }), nil
}

func (s *Service) StatefulSet(ctx context.Context, clusterID int64, namespace, name string) (StatefulSet, error) {
	var item StatefulSet
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/apis/apps/v1", namespace, "statefulsets", name), nil, &item)
	return item, err
}

func (s *Service) DaemonSets(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[DaemonSet], error) {
	path := namespacedListPath("/apis/apps/v1", namespace, "daemonsets")
	var envelope listEnvelope[DaemonSet]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[DaemonSet]{}, err
	}
	return pageResponse(envelope.Items, query, func(item DaemonSet) string { return item.Metadata.Name }), nil
}

func (s *Service) DaemonSet(ctx context.Context, clusterID int64, namespace, name string) (DaemonSet, error) {
	var item DaemonSet
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/apis/apps/v1", namespace, "daemonsets", name), nil, &item)
	return item, err
}

func (s *Service) ReplicaSets(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[ReplicaSet], error) {
	path := namespacedListPath("/apis/apps/v1", namespace, "replicasets")
	var envelope listEnvelope[ReplicaSet]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[ReplicaSet]{}, err
	}
	return pageResponse(envelope.Items, query, func(item ReplicaSet) string { return item.Metadata.Name }), nil
}

func (s *Service) ReplicaSet(ctx context.Context, clusterID int64, namespace, name string) (ReplicaSet, error) {
	var item ReplicaSet
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/apis/apps/v1", namespace, "replicasets", name), nil, &item)
	return item, err
}

func (s *Service) Jobs(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[Job], error) {
	path := namespacedListPath("/apis/batch/v1", namespace, "jobs")
	var envelope listEnvelope[Job]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[Job]{}, err
	}
	return pageResponse(envelope.Items, query, func(item Job) string { return item.Metadata.Name }), nil
}

func (s *Service) Job(ctx context.Context, clusterID int64, namespace, name string) (Job, error) {
	var item Job
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/apis/batch/v1", namespace, "jobs", name), nil, &item)
	return item, err
}

func (s *Service) CronJobs(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[CronJob], error) {
	path := namespacedListPath("/apis/batch/v1", namespace, "cronjobs")
	var envelope listEnvelope[CronJob]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[CronJob]{}, err
	}
	return pageResponse(envelope.Items, query, func(item CronJob) string { return item.Metadata.Name }), nil
}

func (s *Service) CronJob(ctx context.Context, clusterID int64, namespace, name string) (CronJob, error) {
	var item CronJob
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/apis/batch/v1", namespace, "cronjobs", name), nil, &item)
	return item, err
}

func (s *Service) HorizontalPodAutoscalers(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[HorizontalPodAutoscaler], error) {
	path := namespacedListPath("/apis/autoscaling/v2", namespace, "horizontalpodautoscalers")
	var envelope listEnvelope[HorizontalPodAutoscaler]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[HorizontalPodAutoscaler]{}, err
	}
	for index := range envelope.Items {
		normalizeHPA(&envelope.Items[index])
	}
	return pageResponse(envelope.Items, query, func(item HorizontalPodAutoscaler) string { return item.Metadata.Name }), nil
}

func (s *Service) HorizontalPodAutoscaler(ctx context.Context, clusterID int64, namespace, name string) (HorizontalPodAutoscaler, error) {
	var item HorizontalPodAutoscaler
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/apis/autoscaling/v2", namespace, "horizontalpodautoscalers", name), nil, &item)
	normalizeHPA(&item)
	return item, err
}

func (s *Service) ResourceQuotas(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[ResourceQuota], error) {
	path := namespacedListPath("/api/v1", namespace, "resourcequotas")
	var envelope listEnvelope[ResourceQuota]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[ResourceQuota]{}, err
	}
	return pageResponse(envelope.Items, query, func(item ResourceQuota) string { return item.Metadata.Name }), nil
}

func (s *Service) ResourceQuota(ctx context.Context, clusterID int64, namespace, name string) (ResourceQuota, error) {
	var item ResourceQuota
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/api/v1", namespace, "resourcequotas", name), nil, &item)
	return item, err
}

func (s *Service) LimitRanges(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[LimitRange], error) {
	path := namespacedListPath("/api/v1", namespace, "limitranges")
	var envelope listEnvelope[LimitRange]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[LimitRange]{}, err
	}
	for index := range envelope.Items {
		normalizeLimitRange(&envelope.Items[index])
	}
	return pageResponse(envelope.Items, query, func(item LimitRange) string { return item.Metadata.Name }), nil
}

func (s *Service) LimitRange(ctx context.Context, clusterID int64, namespace, name string) (LimitRange, error) {
	var item LimitRange
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/api/v1", namespace, "limitranges", name), nil, &item)
	normalizeLimitRange(&item)
	return item, err
}

func (s *Service) Secrets(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[Secret], error) {
	path := namespacedListPath("/api/v1", namespace, "secrets")
	var envelope listEnvelope[rawSecret]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[Secret]{}, err
	}
	items := make([]Secret, 0, len(envelope.Items))
	for _, item := range envelope.Items {
		items = append(items, sanitizeSecret(item))
	}
	return pageResponse(items, query, func(item Secret) string { return item.Metadata.Name }), nil
}

func (s *Service) Secret(ctx context.Context, clusterID int64, namespace, name string) (Secret, error) {
	var item rawSecret
	if err := s.getJSON(ctx, clusterID, namespacedDetailPath("/api/v1", namespace, "secrets", name), nil, &item); err != nil {
		return Secret{}, err
	}
	return sanitizeSecret(item), nil
}

func (s *Service) Services(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[ServiceResource], error) {
	path := "/api/v1/services"
	if namespace != "" {
		path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/services"
	}
	var envelope listEnvelope[ServiceResource]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[ServiceResource]{}, err
	}
	return pageResponse(envelope.Items, query, func(item ServiceResource) string { return item.Metadata.Name }), nil
}

func (s *Service) GetService(ctx context.Context, clusterID int64, namespace, name string) (ServiceResource, error) {
	var item ServiceResource
	err := s.getJSON(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/services/"+url.PathEscape(name), nil, &item)
	return item, err
}

func (s *Service) Ingresses(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[Ingress], error) {
	path := "/apis/networking.k8s.io/v1/ingresses"
	if namespace != "" {
		path = "/apis/networking.k8s.io/v1/namespaces/" + url.PathEscape(namespace) + "/ingresses"
	}
	var envelope listEnvelope[Ingress]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[Ingress]{}, err
	}
	return pageResponse(envelope.Items, query, func(item Ingress) string { return item.Metadata.Name }), nil
}

func (s *Service) Ingress(ctx context.Context, clusterID int64, namespace, name string) (Ingress, error) {
	var item Ingress
	err := s.getJSON(ctx, clusterID, "/apis/networking.k8s.io/v1/namespaces/"+url.PathEscape(namespace)+"/ingresses/"+url.PathEscape(name), nil, &item)
	return item, err
}

func (s *Service) EndpointSlices(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[EndpointSlice], error) {
	path := "/apis/discovery.k8s.io/v1/endpointslices"
	if namespace != "" {
		path = "/apis/discovery.k8s.io/v1/namespaces/" + url.PathEscape(namespace) + "/endpointslices"
	}
	var envelope listEnvelope[EndpointSlice]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[EndpointSlice]{}, err
	}
	for index := range envelope.Items {
		envelope.Items[index].ServiceName = envelope.Items[index].Metadata.Labels["kubernetes.io/service-name"]
		if envelope.Items[index].Ports == nil {
			envelope.Items[index].Ports = []EndpointSlicePort{}
		}
		if envelope.Items[index].Endpoints == nil {
			envelope.Items[index].Endpoints = []EndpointSliceEndpoint{}
		}
	}
	return pageResponse(envelope.Items, query, func(item EndpointSlice) string { return item.Metadata.Name }), nil
}

func (s *Service) PersistentVolumeClaims(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[PersistentVolumeClaim], error) {
	path := "/api/v1/persistentvolumeclaims"
	if namespace != "" {
		path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/persistentvolumeclaims"
	}
	var envelope listEnvelope[PersistentVolumeClaim]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[PersistentVolumeClaim]{}, err
	}
	return pageResponse(envelope.Items, query, func(item PersistentVolumeClaim) string { return item.Metadata.Name }), nil
}

func (s *Service) PersistentVolumeClaim(ctx context.Context, clusterID int64, namespace, name string) (PersistentVolumeClaim, error) {
	var item PersistentVolumeClaim
	err := s.getJSON(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/persistentvolumeclaims/"+url.PathEscape(name), nil, &item)
	return item, err
}

func (s *Service) StorageClasses(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[StorageClass], error) {
	var envelope listEnvelope[StorageClass]
	if err := s.getJSON(ctx, clusterID, "/apis/storage.k8s.io/v1/storageclasses", selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[StorageClass]{}, err
	}
	return pageResponse(envelope.Items, query, func(item StorageClass) string { return item.Metadata.Name }), nil
}

func (s *Service) StorageClass(ctx context.Context, clusterID int64, name string) (StorageClass, error) {
	var item StorageClass
	err := s.getJSON(ctx, clusterID, "/apis/storage.k8s.io/v1/storageclasses/"+url.PathEscape(name), nil, &item)
	return item, err
}

func (s *Service) PersistentVolumes(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[PersistentVolume], error) {
	var envelope listEnvelope[PersistentVolume]
	if err := s.getJSON(ctx, clusterID, "/api/v1/persistentvolumes", selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[PersistentVolume]{}, err
	}
	return pageResponse(envelope.Items, query, func(item PersistentVolume) string { return item.Metadata.Name }), nil
}

func (s *Service) PersistentVolume(ctx context.Context, clusterID int64, name string) (PersistentVolume, error) {
	var item PersistentVolume
	err := s.getJSON(ctx, clusterID, "/api/v1/persistentvolumes/"+url.PathEscape(name), nil, &item)
	return item, err
}

func (s *Service) PodDisruptionBudgets(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[PodDisruptionBudget], error) {
	path := "/apis/policy/v1/poddisruptionbudgets"
	if namespace != "" {
		path = "/apis/policy/v1/namespaces/" + url.PathEscape(namespace) + "/poddisruptionbudgets"
	}
	var envelope listEnvelope[PodDisruptionBudget]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[PodDisruptionBudget]{}, err
	}
	return pageResponse(envelope.Items, query, func(item PodDisruptionBudget) string { return item.Metadata.Name }), nil
}

func (s *Service) PodDisruptionBudget(ctx context.Context, clusterID int64, namespace, name string) (PodDisruptionBudget, error) {
	var item PodDisruptionBudget
	err := s.getJSON(ctx, clusterID, "/apis/policy/v1/namespaces/"+url.PathEscape(namespace)+"/poddisruptionbudgets/"+url.PathEscape(name), nil, &item)
	return item, err
}

func (s *Service) NetworkPolicies(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[NetworkPolicy], error) {
	path := "/networking.k8s.io/v1/networkpolicies"
	if namespace != "" {
		path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/networkpolicies"
	}
	var envelope listEnvelope[NetworkPolicy]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[NetworkPolicy]{}, err
	}
	return pageResponse(envelope.Items, query, func(item NetworkPolicy) string { return item.Metadata.Name }), nil
}

func (s *Service) NetworkPolicy(ctx context.Context, clusterID int64, namespace, name string) (NetworkPolicy, error) {
	var item NetworkPolicy
	err := s.getJSON(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/networkpolicies/"+url.PathEscape(name), nil, &item)
	return item, err
}

func (s *Service) ServiceAccounts(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[ServiceAccount], error) {
	path := "/api/v1/serviceaccounts"
	if namespace != "" {
		path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/serviceaccounts"
	}
	var envelope listEnvelope[ServiceAccount]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[ServiceAccount]{}, err
	}
	return pageResponse(envelope.Items, query, func(item ServiceAccount) string { return item.Metadata.Name }), nil
}

func (s *Service) ServiceAccount(ctx context.Context, clusterID int64, namespace, name string) (ServiceAccount, error) {
	var item ServiceAccount
	err := s.getJSON(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/serviceaccounts/"+url.PathEscape(name), nil, &item)
	return item, err
}

// normalizeRBACRules ensures Rules and Subject slices are non-nil so callers
// get stable empty arrays instead of null in JSON responses.
func normalizeRBACRules(rules []PolicyRule) []PolicyRule {
	if rules == nil {
		return []PolicyRule{}
	}
	return rules
}

func normalizeSubjects(subjects []Subject) []Subject {
	if subjects == nil {
		return []Subject{}
	}
	return subjects
}

// Roles lists namespaced Kubernetes Role resources. The rbac.authorization.k8s.io
// API group is read-only at this layer; no Role mutation is exposed.
func (s *Service) Roles(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[Role], error) {
	path := namespacedListPath("/apis/rbac.authorization.k8s.io/v1", namespace, "roles")
	var envelope listEnvelope[Role]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[Role]{}, err
	}
	for i := range envelope.Items {
		envelope.Items[i].Rules = normalizeRBACRules(envelope.Items[i].Rules)
	}
	return pageResponse(envelope.Items, query, func(item Role) string { return item.Metadata.Name }), nil
}

// Role reads a single namespaced Kubernetes Role by namespace and name.
func (s *Service) Role(ctx context.Context, clusterID int64, namespace, name string) (Role, error) {
	var item Role
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/apis/rbac.authorization.k8s.io/v1", namespace, "roles", name), nil, &item)
	if err == nil {
		item.Rules = normalizeRBACRules(item.Rules)
	}
	return item, err
}

// ClusterRoles lists cluster-scoped Kubernetes ClusterRole resources.
func (s *Service) ClusterRoles(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[ClusterRole], error) {
	var envelope listEnvelope[ClusterRole]
	if err := s.getJSON(ctx, clusterID, "/apis/rbac.authorization.k8s.io/v1/clusterroles", selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[ClusterRole]{}, err
	}
	for i := range envelope.Items {
		envelope.Items[i].Rules = normalizeRBACRules(envelope.Items[i].Rules)
	}
	return pageResponse(envelope.Items, query, func(item ClusterRole) string { return item.Metadata.Name }), nil
}

// ClusterRole reads a single cluster-scoped Kubernetes ClusterRole by name.
func (s *Service) ClusterRole(ctx context.Context, clusterID int64, name string) (ClusterRole, error) {
	var item ClusterRole
	err := s.getJSON(ctx, clusterID, "/apis/rbac.authorization.k8s.io/v1/clusterroles/"+url.PathEscape(name), nil, &item)
	if err == nil {
		item.Rules = normalizeRBACRules(item.Rules)
	}
	return item, err
}

// RoleBindings lists namespaced Kubernetes RoleBinding resources.
func (s *Service) RoleBindings(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[RoleBinding], error) {
	path := namespacedListPath("/apis/rbac.authorization.k8s.io/v1", namespace, "rolebindings")
	var envelope listEnvelope[RoleBinding]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[RoleBinding]{}, err
	}
	for i := range envelope.Items {
		envelope.Items[i].Subjects = normalizeSubjects(envelope.Items[i].Subjects)
	}
	return pageResponse(envelope.Items, query, func(item RoleBinding) string { return item.Metadata.Name }), nil
}

// RoleBinding reads a single namespaced Kubernetes RoleBinding by namespace and name.
func (s *Service) RoleBinding(ctx context.Context, clusterID int64, namespace, name string) (RoleBinding, error) {
	var item RoleBinding
	err := s.getJSON(ctx, clusterID, namespacedDetailPath("/apis/rbac.authorization.k8s.io/v1", namespace, "rolebindings", name), nil, &item)
	if err == nil {
		item.Subjects = normalizeSubjects(item.Subjects)
	}
	return item, err
}

// ClusterRoleBindings lists cluster-scoped Kubernetes ClusterRoleBinding resources.
func (s *Service) ClusterRoleBindings(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[ClusterRoleBinding], error) {
	var envelope listEnvelope[ClusterRoleBinding]
	if err := s.getJSON(ctx, clusterID, "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings", selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[ClusterRoleBinding]{}, err
	}
	for i := range envelope.Items {
		envelope.Items[i].Subjects = normalizeSubjects(envelope.Items[i].Subjects)
	}
	return pageResponse(envelope.Items, query, func(item ClusterRoleBinding) string { return item.Metadata.Name }), nil
}

// ClusterRoleBinding reads a single cluster-scoped Kubernetes ClusterRoleBinding by name.
func (s *Service) ClusterRoleBinding(ctx context.Context, clusterID int64, name string) (ClusterRoleBinding, error) {
	var item ClusterRoleBinding
	err := s.getJSON(ctx, clusterID, "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/"+url.PathEscape(name), nil, &item)
	if err == nil {
		item.Subjects = normalizeSubjects(item.Subjects)
	}
	return item, err
}

var manifestAllowlist = map[string]bool{
	"Pod":                   true,
	"Deployment":            true,
	"Service":               true,
	"Ingress":               true,
	"PersistentVolumeClaim": true,
	"PersistentVolume":      true,
	"PodDisruptionBudget":   true,
	"NetworkPolicy":         true,
	"ServiceAccount":        true,
	"Role":                  true,
	"ClusterRole":           true,
	"RoleBinding":           true,
	"ClusterRoleBinding":    true,
}

var manifestSensitiveFields = map[string]bool{
	"password":      true,
	"passwd":        true,
	"secret":        true,
	"token":         true,
	"access_key":    true,
	"secret_key":    true,
	"private_key":   true,
	"client_secret": true,
	"api_key":       true,
	"auth":          true,
	"credential":    true,
	"credentials":   true,
	"kubeconfig":    true,
}

func (s *Service) Manifest(ctx context.Context, clusterID int64, kind, namespace, name string) (map[string]interface{}, error) {
	if !manifestAllowlist[kind] {
		return nil, ErrResourceNotFound
	}
	path := manifestPath(kind, namespace, name)
	body, err := s.getRaw(ctx, clusterID, path)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	redactManifest(raw, "")
	return raw, nil
}

func manifestPath(kind, namespace, name string) string {
	switch kind {
	case "Pod":
		return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods/" + url.PathEscape(name)
	case "Service":
		return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/services/" + url.PathEscape(name)
	case "ServiceAccount":
		return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/serviceaccounts/" + url.PathEscape(name)
	case "PersistentVolumeClaim":
		return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/persistentvolumeclaims/" + url.PathEscape(name)
	case "PersistentVolume":
		return "/api/v1/persistentvolumes/" + url.PathEscape(name)
	case "PodDisruptionBudget":
		return "/apis/policy/v1/namespaces/" + url.PathEscape(namespace) + "/poddisruptionbudgets/" + url.PathEscape(name)
	case "NetworkPolicy":
		return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/networkpolicies/" + url.PathEscape(name)
	case "Deployment":
		return "/apis/apps/v1/namespaces/" + url.PathEscape(namespace) + "/deployments/" + url.PathEscape(name)
	case "Ingress":
		return "/apis/networking.k8s.io/v1/namespaces/" + url.PathEscape(namespace) + "/ingresses/" + url.PathEscape(name)
	case "Role":
		return "/apis/rbac.authorization.k8s.io/v1/namespaces/" + url.PathEscape(namespace) + "/roles/" + url.PathEscape(name)
	case "ClusterRole":
		return "/apis/rbac.authorization.k8s.io/v1/clusterroles/" + url.PathEscape(name)
	case "RoleBinding":
		return "/apis/rbac.authorization.k8s.io/v1/namespaces/" + url.PathEscape(namespace) + "/rolebindings/" + url.PathEscape(name)
	case "ClusterRoleBinding":
		return "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/" + url.PathEscape(name)
	default:
		return ""
	}
}

func (s *Service) getRaw(ctx context.Context, clusterID int64, path string) ([]byte, error) {
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	body, err := s.gateway.Get(ctx, clusterID, kubeconfig, path, nil, 10<<20)
	if err != nil {
		return nil, mapGatewayError(err)
	}
	return body, nil
}

func redactManifest(obj map[string]interface{}, prefix string) {
	for key, value := range obj {
		lowerKey := strings.ToLower(key)
		if manifestSensitiveFields[lowerKey] {
			obj[key] = "<redacted>"
			continue
		}
		if key == "data" || key == "stringData" {
			obj[key] = "<redacted>"
			continue
		}
		if key == "secrets" && prefix == ".spec" {
			obj[key] = "<redacted>"
			continue
		}
		if isSensitiveValueKey(lowerKey) {
			if hasSensitiveNameSibling(obj) {
				obj[key] = "<redacted>"
				continue
			}
		}
		switch v := value.(type) {
		case map[string]interface{}:
			redactManifest(v, prefix+"."+key)
		case []interface{}:
			for i, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					redactManifest(m, prefix+"."+key+"["+strconv.Itoa(i)+"]")
				}
			}
		}
	}
}

func isSensitiveValueKey(key string) bool {
	switch key {
	case "value", "default", "optional", "host", "port":
		return true
	}
	return false
}

func hasSensitiveNameSibling(obj map[string]interface{}) bool {
	nameVal, ok := obj["name"]
	if !ok {
		return false
	}
	nameStr, ok := nameVal.(string)
	if !ok {
		return false
	}
	return containsSensitiveWord(nameStr)
}

func containsSensitiveWord(s string) bool {
	lower := strings.ToLower(s)
	for word := range manifestSensitiveFields {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func (s *Service) ConfigMaps(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[ConfigMap], error) {
	path := "/api/v1/configmaps"
	if namespace != "" {
		path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/configmaps"
	}
	var envelope listEnvelope[rawConfigMap]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[ConfigMap]{}, err
	}
	items := make([]ConfigMap, 0, len(envelope.Items))
	for _, item := range envelope.Items {
		items = append(items, sanitizeConfigMap(item))
	}
	return pageResponse(items, query, func(item ConfigMap) string { return item.Metadata.Name }), nil
}

func (s *Service) ConfigMap(ctx context.Context, clusterID int64, namespace, name string) (ConfigMap, error) {
	var item rawConfigMap
	if err := s.getJSON(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/configmaps/"+url.PathEscape(name), nil, &item); err != nil {
		return ConfigMap{}, err
	}
	return sanitizeConfigMap(item), nil
}

func sanitizeConfigMap(item rawConfigMap) ConfigMap {
	return ConfigMap{
		Metadata:       item.Metadata,
		Immutable:      item.Immutable,
		DataKeys:       sortedMapKeys(item.Data),
		BinaryDataKeys: sortedMapKeys(item.BinaryData),
	}
}

func sanitizeSecret(item rawSecret) Secret {
	metadata := ObjectMeta{
		Name:              item.Metadata.Name,
		Namespace:         item.Metadata.Namespace,
		UID:               item.Metadata.UID,
		CreationTimestamp: item.Metadata.CreationTimestamp,
		ResourceVersion:   item.Metadata.ResourceVersion,
	}
	return Secret{
		Metadata:  metadata,
		Immutable: item.Immutable,
		Type:      item.Type,
		DataKeys:  sortedMapKeys(item.Data),
	}
}

func normalizeHPA(item *HorizontalPodAutoscaler) {
	if item.Spec.Metrics == nil {
		item.Spec.Metrics = []HPAMetricSpec{}
	}
	if item.Status.Conditions == nil {
		item.Status.Conditions = []WorkloadCondition{}
	}
}

func normalizeLimitRange(item *LimitRange) {
	if item.Spec.Limits == nil {
		item.Spec.Limits = []LimitRangeItem{}
	}
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func namespacedListPath(apiPrefix, namespace, resource string) string {
	if namespace == "" {
		return apiPrefix + "/" + resource
	}
	return apiPrefix + "/namespaces/" + url.PathEscape(namespace) + "/" + resource
}

func namespacedDetailPath(apiPrefix, namespace, resource, name string) string {
	return apiPrefix + "/namespaces/" + url.PathEscape(namespace) + "/" + resource + "/" + url.PathEscape(name)
}

func (s *Service) ServiceEndpoints(ctx context.Context, clusterID int64, namespace, name string) (Endpoints, error) {
	path := "/apis/discovery.k8s.io/v1/namespaces/" + url.PathEscape(namespace) + "/endpointslices"
	query := url.Values{"labelSelector": {"kubernetes.io/service-name=" + name}}
	var slices listEnvelope[EndpointSlice]
	if err := s.getJSON(ctx, clusterID, path, query, &slices); err == nil {
		return endpointsFromSlices(namespace, name, slices.Items), nil
	} else if !errors.Is(err, ErrResourceNotFound) {
		return Endpoints{}, err
	}
	var item Endpoints
	err := s.getJSON(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/endpoints/"+url.PathEscape(name), nil, &item)
	item.SourceAPI = "core/v1"
	return item, err
}

func endpointsFromSlices(namespace, name string, slices []EndpointSlice) Endpoints {
	result := Endpoints{Metadata: ObjectMeta{Name: name, Namespace: namespace}, SourceAPI: "discovery.k8s.io/v1"}
	for _, slice := range slices {
		subset := EndpointSubset{Ports: make([]EndpointPort, 0, len(slice.Ports))}
		for _, port := range slice.Ports {
			subset.Ports = append(subset.Ports, EndpointPort{Name: port.Name, Port: port.Port, Protocol: port.Protocol})
		}
		for _, endpoint := range slice.Endpoints {
			ready := endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready
			for _, address := range endpoint.Addresses {
				item := EndpointAddress{IP: address, TargetRef: endpoint.TargetRef}
				if endpoint.NodeName != nil {
					item.NodeName = *endpoint.NodeName
				}
				if ready {
					subset.Addresses = append(subset.Addresses, item)
				} else {
					subset.NotReadyAddresses = append(subset.NotReadyAddresses, item)
				}
			}
		}
		result.Subsets = append(result.Subsets, subset)
	}
	return result
}

func (s *Service) Pods(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[Pod], error) {
	path := "/api/v1/pods"
	if namespace != "" {
		path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods"
	}
	var envelope listEnvelope[Pod]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[Pod]{}, err
	}
	total := countNamed(envelope.Items, query.Name, func(item Pod) string { return item.Metadata.Name })
	items := filterAndPage(envelope.Items, query, func(item Pod) string { return item.Metadata.Name })
	return apiquery.ListResponse[Pod]{Items: items, Total: total, Remaining: remaining(len(items), total, query.Offset)}, nil
}

func (s *Service) Pod(ctx context.Context, clusterID int64, namespace, name string) (Pod, error) {
	var item Pod
	err := s.getJSON(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/pods/"+url.PathEscape(name), nil, &item)
	return item, err
}

func (s *Service) Events(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[Event], error) {
	path := "/api/v1/events"
	if namespace != "" {
		path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/events"
	}
	var envelope listEnvelope[Event]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[Event]{}, err
	}
	total := countNamed(envelope.Items, query.Name, func(item Event) string { return item.InvolvedObject.Name })
	items := filterAndPage(envelope.Items, query, func(item Event) string { return item.InvolvedObject.Name })
	return apiquery.ListResponse[Event]{Items: items, Total: total, Remaining: remaining(len(items), total, query.Offset)}, nil
}

func (s *Service) PodEvents(ctx context.Context, clusterID int64, namespace, uid string) ([]Event, error) {
	return s.ResourceEvents(ctx, clusterID, namespace, uid)
}

// ResourceEvents returns Events linked to one exact Kubernetes object UID.
// Names are reusable, so diagnosis rules must not correlate Events by name.
func (s *Service) ResourceEvents(ctx context.Context, clusterID int64, namespace, uid string) ([]Event, error) {
	response, err := s.Events(ctx, clusterID, namespace, apiquery.ListQuery{Page: 1, Limit: 100, FieldSelector: "involvedObject.uid=" + uid})
	return response.Items, err
}

type PodContainerInfo struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	State        string `json:"state"`
	IsInit       bool   `json:"is_init"`
	Image        string `json:"image"`
}

type PodLogLine struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

type PodContainerLog struct {
	Container        string       `json:"container"`
	Lines            []PodLogLine `json:"lines"`
	Truncated        bool         `json:"truncated"`
	TruncationReason string       `json:"truncation_reason,omitempty"`
}

type PodLogsResponse struct {
	Containers []PodContainerLog `json:"containers"`
	Previous   bool              `json:"previous"`
}

func (s *Service) Containers(ctx context.Context, clusterID int64, namespace, name string) ([]PodContainerInfo, error) {
	pod, err := s.Pod(ctx, clusterID, namespace, name)
	if err != nil {
		return nil, err
	}
	result := make([]PodContainerInfo, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers))
	for _, c := range pod.Spec.Containers {
		info := PodContainerInfo{Name: c.Name, Image: c.Image, IsInit: false}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Name == c.Name {
				info.Ready = cs.Ready
				info.RestartCount = cs.RestartCount
				info.State = containerStateLabel(cs.State)
				break
			}
		}
		result = append(result, info)
	}
	for _, c := range pod.Spec.InitContainers {
		info := PodContainerInfo{Name: c.Name, Image: c.Image, IsInit: true}
		for _, cs := range pod.Status.InitContainerStatuses {
			if cs.Name == c.Name {
				info.Ready = cs.Ready
				info.RestartCount = cs.RestartCount
				info.State = containerStateLabel(cs.State)
				break
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func containerStateLabel(state ContainerState) string {
	if state.Waiting != nil {
		return "waiting"
	}
	if state.Terminated != nil {
		return "terminated"
	}
	return "running"
}

func (s *Service) Logs(ctx context.Context, clusterID int64, namespace, name, container string, previous bool, tailLines int) (string, error) {
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return "", err
	}
	query := url.Values{"tailLines": {fmt.Sprintf("%d", tailLines)}, "timestamps": {"true"}}
	if container != "" {
		query.Set("container", container)
	}
	if previous {
		query.Set("previous", "true")
	}
	body, err := s.gateway.Get(ctx, clusterID, kubeconfig, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/pods/"+url.PathEscape(name)+"/log", query, 1<<20)
	return string(body), mapGatewayError(err)
}

func (s *Service) LogsSince(ctx context.Context, clusterID int64, namespace, name, container string, previous bool, tailLines int, sinceSeconds int, sinceTime string) (PodContainerLog, error) {
	if sinceSeconds > 0 && strings.TrimSpace(sinceTime) != "" {
		return PodContainerLog{}, errors.New("sinceSeconds and sinceTime are mutually exclusive")
	}
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return PodContainerLog{}, err
	}
	query := url.Values{"tailLines": {fmt.Sprintf("%d", tailLines)}, "timestamps": {"true"}}
	if container != "" {
		query.Set("container", container)
	}
	if previous {
		query.Set("previous", "true")
	}
	if sinceSeconds > 0 {
		query.Set("sinceSeconds", fmt.Sprintf("%d", sinceSeconds))
	} else if sinceTime != "" {
		query.Set("sinceTime", sinceTime)
	}
	body, err := s.gateway.Get(ctx, clusterID, kubeconfig, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/pods/"+url.PathEscape(name)+"/log", query, 1<<20)
	if err != nil {
		return PodContainerLog{}, mapGatewayError(err)
	}
	lines := parseLogLines(string(body))
	truncated := len(body) >= 1<<20-1
	reason := ""
	if truncated {
		reason = "body_limit"
	}
	return PodContainerLog{
		Container:        container,
		Lines:            lines,
		Truncated:        truncated,
		TruncationReason: reason,
	}, nil
}

func parseLogLines(raw string) []PodLogLine {
	if raw == "" {
		return nil
	}
	segments := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	lines := make([]PodLogLine, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		idx := strings.Index(seg, " ")
		if idx > 0 && seg[0] >= '0' && seg[0] <= '9' {
			lines = append(lines, PodLogLine{Timestamp: seg[:idx], Message: seg[idx+1:]})
		} else {
			lines = append(lines, PodLogLine{Timestamp: "", Message: seg})
		}
	}
	return lines
}

func (s *Service) AllContainerLogs(ctx context.Context, clusterID int64, namespace, name string, previous bool, tailLines int, sinceSeconds int) (PodLogsResponse, error) {
	containers, err := s.Containers(ctx, clusterID, namespace, name)
	if err != nil {
		return PodLogsResponse{}, err
	}
	result := PodLogsResponse{Previous: previous}
	for _, c := range containers {
		log, err := s.LogsSince(ctx, clusterID, namespace, name, c.Name, previous, tailLines, sinceSeconds, "")
		if err != nil {
			log.Container = c.Name
			log.Truncated = true
			log.TruncationReason = "fetch_error"
		}
		result.Containers = append(result.Containers, log)
	}
	return result, nil
}

func (s *Service) getJSON(ctx context.Context, clusterID int64, path string, query url.Values, target any) error {
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return err
	}
	body, err := s.gateway.Get(ctx, clusterID, kubeconfig, path, query, 10<<20)
	if err != nil {
		return mapGatewayError(err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode Kubernetes API response: %w", err)
	}
	return nil
}

func (s *Service) metricsJSON(ctx context.Context, clusterID int64, path string, query url.Values, target any) error {
	err := s.getJSON(ctx, clusterID, path, query, target)
	if !errors.Is(err, ErrResourceNotFound) {
		return err
	}
	var discovery struct{}
	discoveryErr := s.getJSON(ctx, clusterID, "/apis/metrics.k8s.io/v1beta1", nil, &discovery)
	if errors.Is(discoveryErr, ErrResourceNotFound) {
		return ErrMetricsAPIUnavailable
	}
	if discoveryErr != nil {
		return discoveryErr
	}
	return ErrResourceNotFound
}

// veleroJSON mirrors metricsJSON: on a 404 for the resource path, it probes the
// Velero API group root. If the group root also 404s, Velero is not installed;
// otherwise the resource itself is missing.
func (s *Service) veleroJSON(ctx context.Context, clusterID int64, path string, query url.Values, target any) error {
	err := s.getJSON(ctx, clusterID, path, query, target)
	if !errors.Is(err, ErrResourceNotFound) {
		return err
	}
	var discovery struct{}
	discoveryErr := s.getJSON(ctx, clusterID, "/apis/velero.io/v1", nil, &discovery)
	if errors.Is(discoveryErr, ErrResourceNotFound) {
		return ErrVeleroUnavailable
	}
	if discoveryErr != nil {
		return discoveryErr
	}
	return ErrResourceNotFound
}

func mapGatewayError(err error) error {
	var status cluster.APIStatusError
	if errors.As(err, &status) && status.StatusCode == 404 {
		return ErrResourceNotFound
	}
	return err
}

// CreateResource posts a new resource to the Kubernetes API. The body must be a
// complete JSON manifest. When dryRun is true the server validates admission
// without persisting. The path selects the collection, e.g.
// "/apis/apps/v1/namespaces/<ns>/deployments".
func (s *Service) CreateResource(ctx context.Context, clusterID int64, path string, body []byte, dryRun bool) ([]byte, error) {
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	createGateway, ok := s.gateway.(CreateGateway)
	if !ok {
		return nil, errors.New("Kubernetes create gateway is unavailable")
	}
	query := url.Values{}
	if dryRun {
		query.Set("dryRun", "All")
	}
	response, err := createGateway.Create(ctx, clusterID, kubeconfig, path, query, "application/json", body, 10<<20)
	if err != nil {
		return nil, mapCreateGatewayError(err)
	}
	return response, nil
}

// RawManifest returns the unredacted JSON manifest for a resource. It is
// intended for internal promotion use only; the HTTP-facing manifest endpoint
// applies redaction. The kind must be one of the promotion-allowlisted kinds.
func (s *Service) RawManifest(ctx context.Context, clusterID int64, kind, namespace, name string) (json.RawMessage, error) {
	if !validPromotionKind(kind) {
		return nil, fmt.Errorf("unsupported promotion kind %q", kind)
	}
	path, err := promotionPath(kind, namespace, name)
	if err != nil {
		return nil, err
	}
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	body, err := s.gateway.Get(ctx, clusterID, kubeconfig, path, nil, 4<<20)
	if err != nil {
		return nil, mapGatewayError(err)
	}
	return json.RawMessage(body), nil
}

// ResourceExists reports whether a resource at the given path exists. A 404
// returns (false, nil); any other non-2xx returns an error.
func (s *Service) ResourceExists(ctx context.Context, clusterID int64, path string) (bool, error) {
	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		return false, err
	}
	if _, err := s.gateway.Get(ctx, clusterID, kubeconfig, path, nil, 1<<20); err != nil {
		var status cluster.APIStatusError
		if errors.As(err, &status) && status.StatusCode == 404 {
			return false, nil
		}
		return false, mapGatewayError(err)
	}
	return true, nil
}

// NamespaceExists reports whether the Namespace exists on the target cluster.
func (s *Service) NamespaceExists(ctx context.Context, clusterID int64, namespace string) (bool, error) {
	return s.ResourceExists(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace))
}

// ConfigMapExists reports whether the ConfigMap exists on the target cluster.
func (s *Service) ConfigMapExists(ctx context.Context, clusterID int64, namespace, name string) (bool, error) {
	return s.ResourceExists(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/configmaps/"+url.PathEscape(name))
}

// SecretExists reports whether the Secret exists on the target cluster. The
// observer Role grants Secret metadata reads; values are never fetched.
func (s *Service) SecretExists(ctx context.Context, clusterID int64, namespace, name string) (bool, error) {
	return s.ResourceExists(ctx, clusterID, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/secrets/"+url.PathEscape(name))
}

// VeleroCapability probes the Velero API group on the target cluster. A 404
// for /apis/velero.io/v1 means Velero is not installed; the method returns
// {Installed: false} without error. Any other probe failure is propagated.
func (s *Service) VeleroCapability(ctx context.Context, clusterID int64) (VeleroCapability, error) {
	var discovery struct {
		GroupVersion string `json:"groupVersion"`
		Kind         string `json:"kind"`
	}
	err := s.getJSON(ctx, clusterID, "/apis/velero.io/v1", nil, &discovery)
	if errors.Is(err, ErrResourceNotFound) {
		return VeleroCapability{Installed: false}, nil
	}
	if err != nil {
		return VeleroCapability{}, err
	}
	return VeleroCapability{Installed: true, Version: "v1"}, nil
}

// Backups lists Velero Backup CRs on the target cluster. When namespace is
// empty, backups across all namespaces are returned. If the Velero API group
// is not installed, the method returns ErrVeleroUnavailable.
func (s *Service) Backups(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[VeleroBackup], error) {
	path := "/apis/velero.io/v1/backups"
	if namespace != "" {
		path = "/apis/velero.io/v1/namespaces/" + url.PathEscape(namespace) + "/backups"
	}
	var envelope listEnvelope[rawVeleroBackup]
	if err := s.veleroJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[VeleroBackup]{}, err
	}
	projected := make([]VeleroBackup, 0, len(envelope.Items))
	for _, raw := range envelope.Items {
		projected = append(projected, raw.project())
	}
	return pageResponse(projected, query, func(item VeleroBackup) string { return item.Name }), nil
}

// Backup reads a single Velero Backup CR by namespace and name. If the Velero
// API group is not installed, the method returns ErrVeleroUnavailable.
func (s *Service) Backup(ctx context.Context, clusterID int64, namespace, name string) (VeleroBackup, error) {
	var raw rawVeleroBackup
	path := "/apis/velero.io/v1/namespaces/" + url.PathEscape(namespace) + "/backups/" + url.PathEscape(name)
	if err := s.veleroJSON(ctx, clusterID, path, nil, &raw); err != nil {
		return VeleroBackup{}, err
	}
	return raw.project(), nil
}

// BackupStorageLocation is the bounded projection of a Velero BSL CR.
type BackupStorageLocation struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	Provider  string `json:"provider"`
}

type rawBackupStorageLocation struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Provider string `json:"provider"`
	} `json:"spec"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

func (raw rawBackupStorageLocation) project() BackupStorageLocation {
	return BackupStorageLocation{
		Name:      raw.Metadata.Name,
		Namespace: raw.Metadata.Namespace,
		Phase:     raw.Status.Phase,
		Provider:  raw.Spec.Provider,
	}
}

// BackupStorageLocations lists Velero BackupStorageLocation CRs on the target
// cluster. If the Velero API group is not installed, returns ErrVeleroUnavailable.
func (s *Service) BackupStorageLocations(ctx context.Context, clusterID int64, namespace string) ([]BackupStorageLocation, error) {
	path := "/apis/velero.io/v1/backupstoragelocations"
	if namespace != "" {
		path = "/apis/velero.io/v1/namespaces/" + url.PathEscape(namespace) + "/backupstoragelocations"
	}
	var envelope listEnvelope[rawBackupStorageLocation]
	if err := s.veleroJSON(ctx, clusterID, path, nil, &envelope); err != nil {
		return nil, err
	}
	locations := make([]BackupStorageLocation, 0, len(envelope.Items))
	for _, raw := range envelope.Items {
		locations = append(locations, raw.project())
	}
	return locations, nil
}

// VeleroBackupExists reports whether a Velero Backup CR with the given name
// exists in the given namespace on the target cluster.
func (s *Service) VeleroBackupExists(ctx context.Context, clusterID int64, namespace, name string) (bool, error) {
	path := "/apis/velero.io/v1/namespaces/" + url.PathEscape(namespace) + "/backups/" + url.PathEscape(name)
	var raw rawVeleroBackup
	err := s.veleroJSON(ctx, clusterID, path, nil, &raw)
	if errors.Is(err, ErrResourceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// VeleroRestore is the bounded projection of a Velero Restore CR. It carries
// only identity, phase and counts — not the full spec/status.
type VeleroRestore struct {
	Name                string `json:"name"`
	Namespace           string `json:"namespace"`
	UID                 string `json:"uid"`
	ResourceVersion     string `json:"resourceVersion"`
	Phase               string `json:"phase"`
	BackupName          string `json:"backup_name"`
	Errors              int    `json:"errors"`
	Warnings            int    `json:"warnings"`
	FailureReason       string `json:"failure_reason,omitempty"`
	StartTimestamp      string `json:"start_timestamp,omitempty"`
	CompletionTimestamp string `json:"completion_timestamp,omitempty"`
}

type rawVeleroRestore struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		BackupName         string      `json:"backupName"`
		IncludedNamespaces []string    `json:"includedNamespaces,omitempty"`
		NamespaceMapping   interface{} `json:"namespaceMapping,omitempty"`
		RestorePVs         *bool       `json:"restorePVs,omitempty"`
		IncludedResources  []string    `json:"includedResources,omitempty"`
		ExcludedResources  []string    `json:"excludedResources,omitempty"`
	} `json:"spec"`
	Status struct {
		Phase               string `json:"phase,omitempty"`
		FailureReason       string `json:"failureReason,omitempty"`
		Errors              int    `json:"errors,omitempty"`
		Warnings            int    `json:"warnings,omitempty"`
		StartTimestamp      string `json:"startTimestamp,omitempty"`
		CompletionTimestamp string `json:"completionTimestamp,omitempty"`
	} `json:"status"`
}

func (raw rawVeleroRestore) project() VeleroRestore {
	return VeleroRestore{
		Name:                raw.Metadata.Name,
		Namespace:           raw.Metadata.Namespace,
		UID:                 raw.Metadata.UID,
		ResourceVersion:     raw.Metadata.ResourceVersion,
		Phase:               raw.Status.Phase,
		BackupName:          raw.Spec.BackupName,
		Errors:              raw.Status.Errors,
		Warnings:            raw.Status.Warnings,
		FailureReason:       raw.Status.FailureReason,
		StartTimestamp:      raw.Status.StartTimestamp,
		CompletionTimestamp: raw.Status.CompletionTimestamp,
	}
}

// VeleroRestore reads a single Velero Restore CR by namespace and name. If the
// Velero API group is not installed, returns ErrVeleroUnavailable.
func (s *Service) VeleroRestore(ctx context.Context, clusterID int64, namespace, name string) (VeleroRestore, error) {
	var raw rawVeleroRestore
	path := "/apis/velero.io/v1/namespaces/" + url.PathEscape(namespace) + "/restores/" + url.PathEscape(name)
	if err := s.veleroJSON(ctx, clusterID, path, nil, &raw); err != nil {
		return VeleroRestore{}, err
	}
	return raw.project(), nil
}

// VeleroRestoreExists reports whether a Velero Restore CR with the given name
// exists in the given namespace on the target cluster.
func (s *Service) VeleroRestoreExists(ctx context.Context, clusterID int64, namespace, name string) (bool, error) {
	path := "/apis/velero.io/v1/namespaces/" + url.PathEscape(namespace) + "/restores/" + url.PathEscape(name)
	var raw rawVeleroRestore
	err := s.veleroJSON(ctx, clusterID, path, nil, &raw)
	if errors.Is(err, ErrResourceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Restores lists Velero Restore CRs in the given namespace on the target
// cluster. If the Velero API group is not installed, returns ErrVeleroUnavailable.
func (s *Service) Restores(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[VeleroRestore], error) {
	path := "/apis/velero.io/v1/restores"
	if namespace != "" {
		path = "/apis/velero.io/v1/namespaces/" + url.PathEscape(namespace) + "/restores"
	}
	var envelope listEnvelope[rawVeleroRestore]
	if err := s.veleroJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[VeleroRestore]{}, err
	}
	projected := make([]VeleroRestore, 0, len(envelope.Items))
	for _, raw := range envelope.Items {
		projected = append(projected, raw.project())
	}
	return pageResponse(projected, query, func(item VeleroRestore) string { return item.Name }), nil
}

// GitOpsCapability probes the ArgoCD Application API group on the target
// cluster. A 404 for /apis/argoproj.io/v1alpha1 means ArgoCD is not installed;
// the method returns {Installed: false} without error.
type GitOpsCapability struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
}

func (s *Service) GitOpsCapability(ctx context.Context, clusterID int64) (GitOpsCapability, error) {
	var discovery struct {
		GroupVersion string `json:"groupVersion"`
		Kind         string `json:"kind"`
	}
	err := s.getJSON(ctx, clusterID, "/apis/argoproj.io/v1alpha1", nil, &discovery)
	if errors.Is(err, ErrResourceNotFound) {
		return GitOpsCapability{Installed: false}, nil
	}
	if err != nil {
		return GitOpsCapability{}, err
	}
	return GitOpsCapability{Installed: true, Version: "v1alpha1"}, nil
}

// GitOpsApplication is the bounded, read-only projection of an ArgoCD
// Application CR. It carries sync status, health, project, source and
// destination identifiers — not the raw manifest or full spec/status.
type GitOpsApplication struct {
	Name                 string   `json:"name"`
	UID                  string   `json:"uid"`
	ResourceVersion      string   `json:"resource_version"`
	Project              string   `json:"project"`
	SyncStatus           string   `json:"sync_status"`
	SyncRevision         string   `json:"sync_revision,omitempty"`
	HealthStatus         string   `json:"health_status"`
	HealthMessage        string   `json:"health_message,omitempty"`
	SourceRepoURL        string   `json:"source_repo_url"`
	SourceTargetRevision string   `json:"source_target_revision,omitempty"`
	SourcePath           string   `json:"source_path,omitempty"`
	DestinationServer    string   `json:"destination_server,omitempty"`
	DestinationNamespace string   `json:"destination_namespace"`
	OperationStatePhase  string   `json:"operation_state_phase,omitempty"`
	OperationStartedAt   string   `json:"operation_started_at,omitempty"`
	LastSyncStartedAt    string   `json:"last_sync_started_at,omitempty"`
	LastSyncFinishedAt   string   `json:"last_sync_finished_at,omitempty"`
	Conditions           []string `json:"conditions,omitempty"`
	CreatedAt            string   `json:"created_at"`
}

type rawGitOpsApplication struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Project string `json:"project"`
		Source  struct {
			RepoURL        string `json:"repoURL"`
			TargetRevision string `json:"targetRevision,omitempty"`
			Path           string `json:"path,omitempty"`
		} `json:"source"`
		Destination struct {
			Server    string `json:"server,omitempty"`
			Namespace string `json:"namespace"`
		} `json:"destination"`
	} `json:"spec"`
	Status struct {
		Sync struct {
			Status   string `json:"status"`
			Revision string `json:"revision,omitempty"`
		} `json:"sync"`
		Health struct {
			Status  string `json:"status"`
			Message string `json:"message,omitempty"`
		} `json:"health"`
		OperationState struct {
			Phase      string `json:"phase,omitempty"`
			StartedAt  string `json:"startedAt,omitempty"`
			FinishedAt string `json:"finishedAt,omitempty"`
		} `json:"operationState,omitempty"`
		ReconciledAt string `json:"reconciledAt,omitempty"`
		Conditions   []struct {
			Type    string `json:"type"`
			Message string `json:"message,omitempty"`
		} `json:"conditions,omitempty"`
	} `json:"status"`
}

func (raw rawGitOpsApplication) project() GitOpsApplication {
	conds := make([]string, 0, len(raw.Status.Conditions))
	for _, c := range raw.Status.Conditions {
		conds = append(conds, c.Type+": "+c.Message)
	}
	return GitOpsApplication{
		Name:                 raw.Metadata.Name,
		UID:                  raw.Metadata.UID,
		ResourceVersion:      raw.Metadata.ResourceVersion,
		Project:              raw.Spec.Project,
		SyncStatus:           raw.Status.Sync.Status,
		SyncRevision:         raw.Status.Sync.Revision,
		HealthStatus:         raw.Status.Health.Status,
		HealthMessage:        raw.Status.Health.Message,
		SourceRepoURL:        raw.Spec.Source.RepoURL,
		SourceTargetRevision: raw.Spec.Source.TargetRevision,
		SourcePath:           raw.Spec.Source.Path,
		DestinationServer:    raw.Spec.Destination.Server,
		DestinationNamespace: raw.Spec.Destination.Namespace,
		OperationStatePhase:  raw.Status.OperationState.Phase,
		OperationStartedAt:   raw.Status.OperationState.StartedAt,
		LastSyncStartedAt:    raw.Status.OperationState.StartedAt,
		LastSyncFinishedAt:   raw.Status.OperationState.FinishedAt,
		Conditions:           conds,
		CreatedAt:            raw.Metadata.CreationTimestamp,
	}
}

func (s *Service) gitopsJSON(ctx context.Context, clusterID int64, path string, query url.Values, target any) error {
	err := s.getJSON(ctx, clusterID, path, query, target)
	if !errors.Is(err, ErrResourceNotFound) {
		return err
	}
	var discovery struct{}
	discoveryErr := s.getJSON(ctx, clusterID, "/apis/argoproj.io/v1alpha1", nil, &discovery)
	if errors.Is(discoveryErr, ErrResourceNotFound) {
		return ErrGitOpsUnavailable
	}
	if discoveryErr != nil {
		return discoveryErr
	}
	return ErrResourceNotFound
}

// GitOpsApplications lists ArgoCD Application CRs on the target cluster
// (cluster-scoped). If ArgoCD is not installed, returns ErrGitOpsUnavailable.
func (s *Service) GitOpsApplications(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[GitOpsApplication], error) {
	path := "/apis/argoproj.io/v1alpha1/applications"
	var envelope listEnvelope[rawGitOpsApplication]
	if err := s.gitopsJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[GitOpsApplication]{}, err
	}
	projected := make([]GitOpsApplication, 0, len(envelope.Items))
	for _, raw := range envelope.Items {
		projected = append(projected, raw.project())
	}
	return pageResponse(projected, query, func(item GitOpsApplication) string { return item.Name }), nil
}

// GitOpsApplication reads a single ArgoCD Application by name. If ArgoCD is
// not installed, returns ErrGitOpsUnavailable; if name missing returns
// ErrResourceNotFound.
func (s *Service) GitOpsApplication(ctx context.Context, clusterID int64, name string) (GitOpsApplication, error) {
	var raw rawGitOpsApplication
	path := "/apis/argoproj.io/v1alpha1/applications/" + url.PathEscape(name)
	if err := s.gitopsJSON(ctx, clusterID, path, nil, &raw); err != nil {
		return GitOpsApplication{}, err
	}
	return raw.project(), nil
}

func validPromotionKind(kind string) bool {
	switch kind {
	case "Deployment", "Service", "Ingress":
		return true
	}
	return false
}

// promotionPath returns the Kubernetes API path for reading a promotion-allowlisted
// resource. It mirrors the GET paths used by the typed readers.
func promotionPath(kind, namespace, name string) (string, error) {
	switch kind {
	case "Deployment":
		return "/apis/apps/v1/namespaces/" + url.PathEscape(namespace) + "/deployments/" + url.PathEscape(name), nil
	case "Service":
		return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/services/" + url.PathEscape(name), nil
	case "Ingress":
		return "/apis/networking.k8s.io/v1/namespaces/" + url.PathEscape(namespace) + "/ingresses/" + url.PathEscape(name), nil
	}
	return "", fmt.Errorf("unsupported promotion kind %q", kind)
}

func mapCreateGatewayError(err error) error {
	var status cluster.APIStatusError
	if errors.As(err, &status) {
		switch status.StatusCode {
		case 404:
			return ErrResourceNotFound
		case 409:
			return ErrResourceConflict
		}
	}
	return err
}

func selectors(query apiquery.ListQuery) url.Values {
	values := url.Values{}
	if query.LabelSelector != "" {
		values.Set("labelSelector", query.LabelSelector)
	}
	if query.FieldSelector != "" {
		values.Set("fieldSelector", query.FieldSelector)
	}
	return values
}
func countNamed[T any](items []T, name string, getName func(T) string) int {
	count := 0
	for _, item := range items {
		if name == "" || strings.Contains(strings.ToLower(getName(item)), strings.ToLower(name)) {
			count++
		}
	}
	return count
}
func filterAndPage[T any](items []T, query apiquery.ListQuery, getName func(T) string) []T {
	filtered := make([]T, 0, len(items))
	for _, item := range items {
		if query.Name == "" || strings.Contains(strings.ToLower(getName(item)), strings.ToLower(query.Name)) {
			filtered = append(filtered, item)
		}
	}
	if query.SortBy == "name" {
		sort.SliceStable(filtered, func(i, j int) bool {
			if query.Ascending {
				return getName(filtered[i]) < getName(filtered[j])
			}
			return getName(filtered[i]) > getName(filtered[j])
		})
	}
	if query.Offset >= len(filtered) {
		return []T{}
	}
	end := query.Offset + query.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[query.Offset:end]
}
func remaining(returned, total, offset int) int {
	value := total - offset - returned
	if value < 0 {
		return 0
	}
	return value
}

func pageResponse[T any](items []T, query apiquery.ListQuery, getName func(T) string) apiquery.ListResponse[T] {
	total := countNamed(items, query.Name, getName)
	paged := filterAndPage(items, query, getName)
	return apiquery.ListResponse[T]{Items: paged, Total: total, Remaining: remaining(len(paged), total, query.Offset)}
}

// --- Copy / GitOps helpers ---------------------------------------------------

// KindToGVR maps a human-friendly kind name to the canonical (group, version,
// resource) tuple used by the Kubernetes API. It returns ok=false when the
// kind is not in the operator-curated cross-cluster copy whitelist.
func KindToGVR(kind string) (group, version, resource string, ok bool) {
	switch kind {
	case KindDeployment:
		return "apps", "v1", "deployments", true
	case KindStatefulSet:
		return "apps", "v1", "statefulsets", true
	case KindDaemonSet:
		return "apps", "v1", "daemonsets", true
	case KindCronJob:
		return "batch", "v1", "cronjobs", true
	case KindService:
		return "", "v1", "services", true
	case KindIngress:
		return "networking.k8s.io", "v1", "ingresses", true
	case KindServiceAccount:
		return "", "v1", "serviceaccounts", true
	case KindConfigMap:
		return "", "v1", "configmaps", true
	case KindSecret:
		return "", "v1", "secrets", true
	}
	return "", "", "", false
}

const (
	KindDeployment     = "Deployment"
	KindStatefulSet    = "StatefulSet"
	KindDaemonSet      = "DaemonSet"
	KindCronJob        = "CronJob"
	KindService        = "Service"
	KindIngress        = "Ingress"
	KindServiceAccount = "ServiceAccount"
	KindConfigMap      = "ConfigMap"
	KindSecret         = "Secret"
)

func namespacedAPIPath(group, version, resource, namespace, name string) string {
	if group == "" {
		// core API group lives under /api.
		return "/api/" + version + "/namespaces/" + url.PathEscape(namespace) + "/" + resource + "/" + url.PathEscape(name)
	}
	return "/apis/" + group + "/" + version + "/namespaces/" + url.PathEscape(namespace) + "/" + resource + "/" + url.PathEscape(name)
}

//nolint:unused // reserved for future raw resource collection operations
func namespacedCollectionPath(group, version, resource, namespace string) string {
	if group == "" {
		return "/api/" + version + "/namespaces/" + url.PathEscape(namespace) + "/" + resource
	}
	return "/apis/" + group + "/" + version + "/namespaces/" + url.PathEscape(namespace) + "/" + resource
}

// GetRawResource reads a single namespaced Kubernetes resource as an arbitrary
// JSON object (no typed projection). Used by M58 cross-cluster copy, which
// must scrub and re-apply the raw manifest.
func (s *Service) GetRawResource(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (map[string]any, error) {
	if clusterID < 1 {
		return nil, ErrResourceNotFound
	}
	path := namespacedAPIPath(group, version, resource, namespace, name)
	var obj map[string]any
	if err := s.getJSON(ctx, clusterID, path, nil, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// NamespaceExists reports whether a namespace exists on the target cluster.
// A 404 for the namespaced core API path is treated as "does not exist" with
// ok=false and no error.
//
// SourceNamespaceIdentity fetches the source namespace's UID and
// resourceVersion so a copy plan can capture a Compare-And-Swap identity: if
// the source namespace is recreated between Preview and Execute, the Execute
// phase aborts (the operator must re-run Preview).
type SourceNamespaceIdentity struct {
	Name            string `json:"name"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resource_version"`
}

func (s *Service) SourceNamespaceIdentity(ctx context.Context, clusterID int64, namespace string) (SourceNamespaceIdentity, error) {
	if clusterID < 1 {
		return SourceNamespaceIdentity{}, ErrResourceNotFound
	}
	path := "/api/v1/namespaces/" + url.PathEscape(namespace)
	var obj struct {
		Metadata ObjectMeta `json:"metadata"`
	}
	if err := s.getJSON(ctx, clusterID, path, nil, &obj); err != nil {
		return SourceNamespaceIdentity{}, err
	}
	return SourceNamespaceIdentity{
		Name:            obj.Metadata.Name,
		UID:             obj.Metadata.UID,
		ResourceVersion: obj.Metadata.ResourceVersion,
	}, nil
}

// NamespacedResourceExists reports whether a single namespaced object already
// exists on the target cluster. Used by copy preview as a preflight before
// dry-run, so we can skip already-existing resources with a friendly reason
// instead of surfacing a 409 from the dry-run admission.
func (s *Service) NamespacedResourceExists(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (bool, error) {
	if clusterID < 1 {
		return false, nil
	}
	path := namespacedAPIPath(group, version, resource, namespace, name)
	var obj map[string]any
	err := s.getJSON(ctx, clusterID, path, nil, &obj)
	if errors.Is(err, ErrResourceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ============================================================================
// M47: API resource discovery (CRD preview)
// ============================================================================

// ErrDiscoveryUnavailable is returned by APIResources when the Service was
// constructed without a DiscoveryProvider (e.g. in route-contract tests).
var ErrDiscoveryUnavailable = errors.New("kubernetes discovery is unavailable")

// APIResource describes a single (group, version, resource, namespaced, kind)
// tuple discoverable on a cluster. It is the union of the fixed GVR whitelist
// (always present, even if discovery fails) and the dynamically discovered
// resources.
type APIResource struct {
	Group      string `json:"group,omitempty"`
	Version    string `json:"version"`
	Resource   string `json:"resource"`
	Kind       string `json:"kind"`
	Namespaced bool   `json:"namespaced"`
	// Source is "whitelist" for the fixed operator GVRs and "discovery" for
	// CRDs and other dynamically discovered resources. The frontend uses it
	// to distinguish "guaranteed core resources" from "cluster-specific CRDs".
	Source string `json:"source"`
}

// fixedAPIResources is the operator-curated whitelist of GVRs the console
// always renders regardless of discovery success. It mirrors the resource
// families already exposed by the typed list methods on Service (Pods,
// Deployments, Services, ...). Discovery may add CRDs and other dynamic
// resources on top, but never replaces these.
//
// The whitelist is intentionally a static slice — M47 does NOT introduce a
// dynamic GVR proxy (ADR 0062 §3). M49 will refine the CRD subset.
var fixedAPIResources = []APIResource{
	// core/v1
	{Version: "v1", Resource: "pods", Kind: "Pod", Namespaced: true, Source: "whitelist"},
	{Version: "v1", Resource: "services", Kind: "Service", Namespaced: true, Source: "whitelist"},
	{Version: "v1", Resource: "configmaps", Kind: "ConfigMap", Namespaced: true, Source: "whitelist"},
	{Version: "v1", Resource: "secrets", Kind: "Secret", Namespaced: true, Source: "whitelist"},
	{Version: "v1", Resource: "namespaces", Kind: "Namespace", Namespaced: false, Source: "whitelist"},
	{Version: "v1", Resource: "nodes", Kind: "Node", Namespaced: false, Source: "whitelist"},
	{Version: "v1", Resource: "events", Kind: "Event", Namespaced: true, Source: "whitelist"},
	{Version: "v1", Resource: "persistentvolumes", Kind: "PersistentVolume", Namespaced: false, Source: "whitelist"},
	{Version: "v1", Resource: "persistentvolumeclaims", Kind: "PersistentVolumeClaim", Namespaced: true, Source: "whitelist"},
	{Version: "v1", Resource: "serviceaccounts", Kind: "ServiceAccount", Namespaced: true, Source: "whitelist"},
	{Version: "v1", Resource: "resourcequotas", Kind: "ResourceQuota", Namespaced: true, Source: "whitelist"},
	{Version: "v1", Resource: "limitranges", Kind: "LimitRange", Namespaced: true, Source: "whitelist"},
	// apps/v1
	{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true, Source: "whitelist"},
	{Group: "apps", Version: "v1", Resource: "replicasets", Kind: "ReplicaSet", Namespaced: true, Source: "whitelist"},
	{Group: "apps", Version: "v1", Resource: "statefulsets", Kind: "StatefulSet", Namespaced: true, Source: "whitelist"},
	{Group: "apps", Version: "v1", Resource: "daemonsets", Kind: "DaemonSet", Namespaced: true, Source: "whitelist"},
	// batch/v1
	{Group: "batch", Version: "v1", Resource: "jobs", Kind: "Job", Namespaced: true, Source: "whitelist"},
	{Group: "batch", Version: "v1", Resource: "cronjobs", Kind: "CronJob", Namespaced: true, Source: "whitelist"},
	// networking.k8s.io/v1
	{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses", Kind: "Ingress", Namespaced: true, Source: "whitelist"},
	{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies", Kind: "NetworkPolicy", Namespaced: true, Source: "whitelist"},
	// discovery.k8s.io/v1
	{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices", Kind: "EndpointSlice", Namespaced: true, Source: "whitelist"},
	// policy/v1
	{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets", Kind: "PodDisruptionBudget", Namespaced: true, Source: "whitelist"},
	// autoscaling/v2
	{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers", Kind: "HorizontalPodAutoscaler", Namespaced: true, Source: "whitelist"},
	// rbac.authorization.k8s.io/v1
	{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles", Kind: "Role", Namespaced: true, Source: "whitelist"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings", Kind: "RoleBinding", Namespaced: true, Source: "whitelist"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles", Kind: "ClusterRole", Namespaced: false, Source: "whitelist"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings", Kind: "ClusterRoleBinding", Namespaced: false, Source: "whitelist"},
	// storage.k8s.io/v1
	{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses", Kind: "StorageClass", Namespaced: false, Source: "whitelist"},
}

// APIResources returns the union of the fixed GVR whitelist and the
// dynamically discovered resources on the cluster. Discovery failures are
// non-fatal: the whitelist is always returned, and any error from the
// discovery API is swallowed (the discovery source is simply omitted).
//
// This is the M47 preview of M49's full CRD browsing — the response is
// read-only and contains no resource instances, only GVR metadata.
func (s *Service) APIResources(ctx context.Context, clusterID int64) ([]APIResource, error) {
	// Seed with the whitelist so callers always get a usable catalog even
	// when discovery is unavailable (e.g. air-gapped clusters, stale
	// kubeconfig, throttled API server).
	out := make([]APIResource, len(fixedAPIResources))
	copy(out, fixedAPIResources)
	// Sort immediately so the early-return fallback paths (nil discovery,
	// credential error, discovery error) also return a sorted catalog. The
	// discovery-success path re-sorts below after appending dynamic entries.
	sortAPIResources(out)

	if s.discovery == nil {
		return out, nil
	}

	_, kubeconfig, err := s.credentials.Access(ctx, clusterID)
	if err != nil {
		// Whitelist-only fallback; do not surface credential errors here so
		// the endpoint degrades gracefully.
		return out, nil
	}
	disco, err := s.discovery.Discovery(clusterID, kubeconfig)
	if err != nil {
		return out, nil
	}
	// ServerGroupsAndResources is the most complete discovery call. It is
	// expensive (one request per group) but the endpoint is bounded by
	// cluster scope and cached client-side by client-go's discovery memcache.
	_, lists, err := disco.ServerGroupsAndResources()
	if err != nil {
		// Partial discovery is acceptable — return whitelist only.
		return out, nil
	}

	seen := make(map[string]struct{}, len(out))
	for _, r := range out {
		seen[apiResourceKey(r.Group, r.Version, r.Resource)] = struct{}{}
	}

	for _, list := range lists {
		if list == nil {
			continue
		}
		gv := strings.SplitN(list.GroupVersion, "/", 2)
		var group, version string
		if len(gv) == 2 {
			group, version = gv[0], gv[1]
		} else {
			version = gv[0]
		}
		for _, ar := range list.APIResources {
			// Skip subresources (e.g. "pods/log") — they are not listable.
			if strings.Contains(ar.Name, "/") {
				continue
			}
			// Skip verbs that are not listable/gettable; discovery returns
			// resources with no verbs in some cases (e.g. custom subresources).
			if !hasVerb(ar.Verbs, "list") && !hasVerb(ar.Verbs, "get") {
				continue
			}
			key := apiResourceKey(group, version, ar.Name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, APIResource{
				Group:      group,
				Version:    version,
				Resource:   ar.Name,
				Kind:       ar.Kind,
				Namespaced: ar.Namespaced,
				Source:     "discovery",
			})
		}
	}

	sortAPIResources(out)
	return out, nil
}

// sortAPIResources orders resources by group, version, then resource for
// stable frontend rendering. It is a total order, so repeated calls are
// idempotent.
func sortAPIResources(out []APIResource) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}
		return out[i].Resource < out[j].Resource
	})
}

func apiResourceKey(group, version, resource string) string {
	return group + "/" + version + "/" + resource
}

func hasVerb(verbs []string, want string) bool {
	for _, v := range verbs {
		if v == want {
			return true
		}
	}
	return false
}

// ============================================================================
// M49: CRD discovery + read-only custom resource browsing
// ============================================================================

// ErrCustomResourceNotWhitelisted is returned when the requested
// (group, version, resource) tuple is not in the operator-curated
// customResourceWhitelist. The handler maps it to 404 RESOURCE_NOT_FOUND so a
// non-whitelisted CRD is indistinguishable from a missing one — the
// anti-leakage pattern (ADR 0064 §4).
var ErrCustomResourceNotWhitelisted = errors.New("custom resource is not whitelisted for browsing")

// customResourceDescriptor describes a whitelisted CRD GVR. Namespaced reports
// whether instances live in a namespace (true) or are cluster-scoped (false);
// it drives both the API path construction and the handler's fan-out decision.
type customResourceDescriptor struct {
	Namespaced bool
}

// customResourceWhitelist is the operator-curated, compile-time-fixed catalogue
// of CRD GVRs that the console may browse read-only (ADR 0064 §2). It covers
// the common operator CRDs (Velero, Prometheus operator, Flux Helm/source,
// cert-manager). Adding an entry is a contract change — there is no runtime
// discovery-based expansion, preserving the static-extension hard constraint
// and deterministic browseability.
//
// The whitelist is keyed by "group/version/resource". The caller supplies all
// three via the route path, and the exact tuple must match. Core resources
// (empty group) are intentionally absent — they are already covered by the
// typed list endpoints and the M47 fixedAPIResources whitelist.
var customResourceWhitelist = map[string]customResourceDescriptor{
	// Velero (backup/restore).
	"velero.io/v1/backups":   {Namespaced: true},
	"velero.io/v1/restores":  {Namespaced: true},
	"velero.io/v1/schedules": {Namespaced: true},
	// Prometheus operator.
	"monitoring.coreos.com/v1/prometheuses":        {Namespaced: true},
	"monitoring.coreos.com/v1/alertmanagers":       {Namespaced: true},
	"monitoring.coreos.com/v1/servicemonitors":     {Namespaced: true},
	"monitoring.coreos.com/v1/podmonitors":         {Namespaced: true},
	"monitoring.coreos.com/v1/prometheusrules":     {Namespaced: true},
	"monitoring.coreos.com/v1/thanosrulers":        {Namespaced: true},
	"monitoring.coreos.com/v1/probes":              {Namespaced: true},
	"monitoring.coreos.com/v1/alertmanagerconfigs": {Namespaced: true},
	// Flux Helm release + source controllers.
	"helm.toolkit.fluxcd.io/v2beta1/helmreleases":  {Namespaced: true},
	"source.toolkit.fluxcd.io/v1/helmrepositories": {Namespaced: true},
	"source.toolkit.fluxcd.io/v1/gitrepositories":  {Namespaced: true},
	"source.toolkit.fluxcd.io/v1/buckets":          {Namespaced: true},
	"source.toolkit.fluxcd.io/v1/ocirepositories":  {Namespaced: true},
	// cert-manager.
	"cert-manager.io/v1/certificates":        {Namespaced: true},
	"cert-manager.io/v1/certificaterequests": {Namespaced: true},
	"cert-manager.io/v1/issuers":             {Namespaced: true},
	"cert-manager.io/v1/clusterissuers":      {Namespaced: false},
	"cert-manager.io/v1/orders":              {Namespaced: true},
	"cert-manager.io/v1/challenges":          {Namespaced: true},
	// ArgoCD (GitOps, M58). Read-only Application browse. The
	// Application is cluster-scoped in ArgoCD v2.
	"argoproj.io/v1alpha1/applications": {Namespaced: false},
}

// IsCustomResourceBrowsable reports whether the given (group, version, resource)
// tuple is in the operator-curated whitelist for read-only CRD browsing. When
// ok is true, namespaced indicates whether instances live in a namespace (true)
// or are cluster-scoped (false). When ok is false the GVR is not browsable and
// the handler returns 404 (anti-leakage, ADR 0064 §4).
func (s *Service) IsCustomResourceBrowsable(group, version, resource string) (namespaced, ok bool) {
	entry, found := customResourceWhitelist[customResourceKey(group, version, resource)]
	if !found {
		return false, false
	}
	return entry.Namespaced, true
}

func customResourceKey(group, version, resource string) string {
	return group + "/" + version + "/" + resource
}

// CustomResources lists instances of a whitelisted custom resource on a
// cluster. Each returned item is the full manifest with sensitive fields
// redacted via redactManifest (M22 redaction reused — ADR 0064 §3). The list is
// bounded by apiquery.ListQuery (limit ≤100, enforced by apiquery.Parse).
//
// For namespaced CRDs, namespace selects the namespace ("" means all
// namespaces — the handler fans out per authorized namespace for non-cluster
// callers). For cluster-scoped CRDs, namespace is ignored.
//
// Read-only: no write path is exposed (ADR 0064 §2).
func (s *Service) CustomResources(ctx context.Context, clusterID int64, group, version, resource, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[map[string]interface{}], error) {
	desc, ok := customResourceWhitelist[customResourceKey(group, version, resource)]
	if !ok {
		return apiquery.ListResponse[map[string]interface{}]{}, ErrCustomResourceNotWhitelisted
	}
	path := customResourceListPath(group, version, resource, namespace, desc.Namespaced)
	var envelope listEnvelope[map[string]interface{}]
	if err := s.getJSON(ctx, clusterID, path, selectors(query), &envelope); err != nil {
		return apiquery.ListResponse[map[string]interface{}]{}, err
	}
	for i := range envelope.Items {
		if envelope.Items[i] != nil {
			redactManifest(envelope.Items[i], "")
		}
	}
	items := filterAndPage(envelope.Items, query, customResourceName)
	total := countNamed(envelope.Items, query.Name, customResourceName)
	return apiquery.ListResponse[map[string]interface{}]{Items: items, Total: total, Remaining: remaining(len(items), total, query.Offset)}, nil
}

// CustomResource returns a single whitelisted custom resource instance by
// name. The manifest is redacted via redactManifest (M22 redaction reused).
// For namespaced CRDs, namespace must be non-empty; for cluster-scoped CRDs,
// namespace is ignored. Read-only (ADR 0064 §2).
func (s *Service) CustomResource(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (map[string]interface{}, error) {
	desc, ok := customResourceWhitelist[customResourceKey(group, version, resource)]
	if !ok {
		return nil, ErrCustomResourceNotWhitelisted
	}
	path := customResourcePath(group, version, resource, namespace, name, desc.Namespaced)
	var raw map[string]interface{}
	if err := s.getJSON(ctx, clusterID, path, nil, &raw); err != nil {
		return nil, err
	}
	redactManifest(raw, "")
	return raw, nil
}

// customResourceListPath builds the Kubernetes API collection path for a CRD.
// For namespaced CRDs with a non-empty namespace, the path is namespace-scoped;
// otherwise (cluster-scoped CRD, or namespaced CRD with empty namespace to list
// across all namespaces) the cluster-wide collection path is returned.
func customResourceListPath(group, version, resource, namespace string, namespaced bool) string {
	if namespaced && namespace != "" {
		return "/apis/" + url.PathEscape(group) + "/" + url.PathEscape(version) + "/namespaces/" + url.PathEscape(namespace) + "/" + url.PathEscape(resource)
	}
	return "/apis/" + url.PathEscape(group) + "/" + url.PathEscape(version) + "/" + url.PathEscape(resource)
}

// customResourcePath builds the Kubernetes API item path for a single CRD
// instance. For namespaced CRDs the namespace segment is always included
// (callers must supply a non-empty namespace); for cluster-scoped CRDs the
// namespace segment is omitted.
func customResourcePath(group, version, resource, namespace, name string, namespaced bool) string {
	if namespaced {
		return "/apis/" + url.PathEscape(group) + "/" + url.PathEscape(version) + "/namespaces/" + url.PathEscape(namespace) + "/" + url.PathEscape(resource) + "/" + url.PathEscape(name)
	}
	return "/apis/" + url.PathEscape(group) + "/" + url.PathEscape(version) + "/" + url.PathEscape(resource) + "/" + url.PathEscape(name)
}

// customResourceName extracts metadata.name from a raw CRD manifest for
// filterAndPage / countNamed.
func customResourceName(item map[string]interface{}) string {
	if metadata, ok := item["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			return name
		}
	}
	return ""
}
