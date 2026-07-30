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

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
)

var (
	ErrResourceNotFound      = errors.New("Kubernetes resource not found")
	ErrResourceConflict      = errors.New("Kubernetes resource already exists")
	ErrMetricsAPIUnavailable = errors.New("Kubernetes Metrics API is unavailable")
	ErrVeleroUnavailable     = errors.New("Velero API is not installed")
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

type Service struct {
	credentials CredentialSource
	gateway     Gateway
}

func NewService(credentials CredentialSource, gateway Gateway) *Service {
	return &Service{credentials: credentials, gateway: gateway}
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
		Template struct {
			Metadata ObjectMeta `json:"metadata"`
			Spec     struct {
				Containers []struct {
					Name  string `json:"name"`
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
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
	Name  string `json:"name"`
	Image string `json:"image"`
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
		MinAvailable   interface{} `json:"minAvailable,omitempty"`
		MaxUnavailable interface{} `json:"maxUnavailable,omitempty"`
		Selector       interface{} `json:"selector,omitempty"`
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
	Name               string   `json:"name"`
	Namespace          string   `json:"namespace"`
	Phase              string   `json:"phase"`
	IncludedNamespaces []string `json:"included_namespaces,omitempty"`
	StorageLocation    string   `json:"storage_location,omitempty"`
	TTL                string   `json:"ttl,omitempty"`
	Expiration         string   `json:"expiration,omitempty"`
	StartedAt          string   `json:"started_at,omitempty"`
	CompletedAt        string   `json:"completed_at,omitempty"`
	FailureReason      string   `json:"failure_reason,omitempty"`
	Errors             int      `json:"errors"`
	Warnings           int      `json:"warnings"`
	CreatedAt          string   `json:"created_at"`
}

// rawVeleroBackup is the raw Velero Backup CR shape used for decoding. It is
// not exposed to HTTP callers; VeleroBackup is the public projection.
type rawVeleroBackup struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		IncludedNamespaces []string    `json:"includedNamespaces,omitempty"`
		StorageLocation    string      `json:"storageLocation,omitempty"`
		SnapshotVolumes    *bool       `json:"snapshotVolumes,omitempty"`
		TTL                string      `json:"ttl,omitempty"`
		LabelSelector      interface{} `json:"labelSelector,omitempty"`
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
		Name:               raw.Metadata.Name,
		Namespace:          raw.Metadata.Namespace,
		Phase:              raw.Status.Phase,
		IncludedNamespaces: raw.Spec.IncludedNamespaces,
		StorageLocation:    raw.Spec.StorageLocation,
		TTL:                raw.Spec.TTL,
		Expiration:         raw.Status.Expiration,
		StartedAt:          raw.Status.StartTimestamp,
		CompletedAt:        raw.Status.CompletionTimestamp,
		FailureReason:      raw.Status.FailureReason,
		Errors:             raw.Status.Errors,
		Warnings:           raw.Status.Warnings,
		CreatedAt:          raw.Metadata.CreationTimestamp,
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
type Pod struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		NodeName   string `json:"nodeName,omitempty"`
		Containers []struct {
			Name  string `json:"name"`
			Image string `json:"image"`
		} `json:"containers"`
		InitContainers []struct {
			Name  string `json:"name"`
			Image string `json:"image"`
		} `json:"initContainers,omitempty"`
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

// promotionCreatePath returns the Kubernetes API collection path for creating a
// promotion-allowlisted resource in a namespace.
func promotionCreatePath(kind, namespace string) (string, error) {
	switch kind {
	case "Deployment":
		return "/apis/apps/v1/namespaces/" + url.PathEscape(namespace) + "/deployments", nil
	case "Service":
		return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/services", nil
	case "Ingress":
		return "/apis/networking.k8s.io/v1/namespaces/" + url.PathEscape(namespace) + "/ingresses", nil
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
