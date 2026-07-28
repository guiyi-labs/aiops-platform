package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
)

var (
	ErrResourceNotFound      = errors.New("Kubernetes resource not found")
	ErrMetricsAPIUnavailable = errors.New("Kubernetes Metrics API is unavailable")
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
		Replicas            int32 `json:"replicas"`
		ReadyReplicas       int32 `json:"readyReplicas"`
		AvailableReplicas   int32 `json:"availableReplicas"`
		UpdatedReplicas     int32 `json:"updatedReplicas"`
		UnavailableReplicas int32 `json:"unavailableReplicas"`
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
	} `json:"spec"`
	Status struct {
		Phase             string            `json:"phase"`
		PodIP             string            `json:"podIP,omitempty"`
		HostIP            string            `json:"hostIP,omitempty"`
		Reason            string            `json:"reason,omitempty"`
		Message           string            `json:"message,omitempty"`
		Conditions        []PodCondition    `json:"conditions,omitempty"`
		ContainerStatuses []ContainerStatus `json:"containerStatuses,omitempty"`
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

func mapGatewayError(err error) error {
	var status cluster.APIStatusError
	if errors.As(err, &status) && status.StatusCode == 404 {
		return ErrResourceNotFound
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
