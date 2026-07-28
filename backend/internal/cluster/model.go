package cluster

import "time"

const (
	StatusDisabled    = "disabled"
	StatusUnknown     = "unknown"
	StatusReady       = "ready"
	StatusUnreachable = "unreachable"

	ConditionReady           = "Ready"
	ConditionCredentialValid = "CredentialValid"
	ConditionReachable       = "Reachable"
)

type Cluster struct {
	ID                int64       `gorm:"primaryKey" json:"id"`
	Name              string      `gorm:"size:128;uniqueIndex;not null" json:"name"`
	APIServer         string      `gorm:"size:512;not null" json:"api_server"`
	Enabled           bool        `gorm:"not null" json:"enabled"`
	Status            string      `gorm:"size:32;not null" json:"status"`
	KubernetesVersion string      `gorm:"size:64" json:"kubernetes_version,omitempty"`
	LastProbedAt      *time.Time  `json:"last_probed_at,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	Conditions        []Condition `gorm:"foreignKey:ClusterID" json:"conditions"`
}

func (Cluster) TableName() string { return "clusters" }

type Credential struct {
	ClusterID            int64  `gorm:"primaryKey"`
	EncryptedKubeconfig  []byte `gorm:"column:encrypted_kubeconfig;not null"`
	EncryptionKeyVersion string `gorm:"size:64;not null"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (Credential) TableName() string { return "cluster_credentials" }

type Condition struct {
	ClusterID          int64     `gorm:"primaryKey" json:"-"`
	Type               string    `gorm:"primaryKey;size:64" json:"type"`
	Status             string    `gorm:"size:16;not null" json:"status"`
	Reason             string    `gorm:"size:128;not null" json:"reason"`
	Message            string    `gorm:"size:1024;not null" json:"message"`
	LastTransitionTime time.Time `json:"last_transition_time"`
}

func (Condition) TableName() string { return "cluster_conditions" }
