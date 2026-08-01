package appcatalog

import (
	"encoding/json"
	"errors"
	"time"
)

// M57: Helm application catalog + controlled deploy plans.
//
// This package implements the "simplified Helm application catalog" from
// the post-M45 roadmap (M57). It provides:
//
//  1. Helm repository registration (CRUD on helm_repositories).
//  2. Chart listing/search/detail by fetching and parsing each repo's
//     index.yaml over HTTP (read-only, no Helm SDK).
//  3. Controlled chart deployment via the M19 controlled-operation
//     contract: preview builds a Flux HelmRelease CR manifest and
//     validates it via server-side dry-run; execute creates the CR on
//     the target cluster through the bounded kubernetes gateway.
//
// Design constraints (project invariants):
//   - No Helm SDK dependency; chart metadata comes from index.yaml HTTP
//     fetch. The deploy target is a Flux HelmRelease CR (already in the
//     M49 CRD whitelist), not a direct helm install.
//   - Credentials in helm_repositories.credentials_json are NEVER returned
//     in API responses; the handler redacts them (ADR 0008 / project
//     invariant: credentials never in API/UI/logs/audit).
//   - The controlled deploy reuses the M19 confirmation-token +
//     idempotency-key + Claim state machine (mirrors promotion/backup/
//     restore/maintenance).
//   - 404 > 403 anti-leakage: unauthorized repo/plan access returns 404.

const (
	StatusAwaitingConfirmation = "awaiting_confirmation"
	StatusExecuting            = "executing"
	StatusSucceeded            = "succeeded"
	StatusFailed               = "failed"
	StatusExpired              = "expired"

	// HelmRelease CRD constants (Flux helm-controller).
	helmReleaseAPIVersion = "helm.toolkit.fluxcd.io/v2beta1"
	helmReleaseKind       = "HelmRelease"
	helmReleasePlural     = "helmreleases"
)

var (
	ErrRepoNotFound        = errors.New("helm repository not found")
	ErrRepoNameExists      = errors.New("helm repository name already exists")
	ErrRepoURLInvalid      = errors.New("helm repository URL is invalid")
	ErrRepoUnreachable     = errors.New("helm repository is unreachable")
	ErrChartNotFound       = errors.New("chart not found in repository")
	ErrPlanNotFound        = errors.New("app catalog plan not found")
	ErrInvalidRequest      = errors.New("app catalog request is invalid")
	ErrNamespaceMissing    = errors.New("target namespace does not exist")
	ErrClusterUnavailable  = errors.New("target cluster is unavailable")
	ErrPreviewFailed       = errors.New("app catalog dry-run preview failed")
	ErrConfirmationInvalid = errors.New("app catalog confirmation is invalid")
	ErrInvalidIdempotency  = errors.New("app catalog idempotency key is invalid")
	ErrExpired             = errors.New("app catalog plan expired")
	ErrInProgress          = errors.New("app catalog execution is in progress")
	ErrAlreadyExecuted     = errors.New("app catalog plan already used with another idempotency key")
	ErrExecutionFailed     = errors.New("app catalog execution failed")
)

// ActorRef mirrors promotion.ActorRef so the HTTP layer can pass through
// the authenticated operator without importing another package.
type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Repository is a registered Helm repository. The CredentialsJSON field
// stores basic-auth / TLS material as JSONB and is NEVER serialised in
// API responses (the handler redacts it to an empty object).
type Repository struct {
	ID              int64     `gorm:"primaryKey" json:"id"`
	Name            string    `gorm:"column:name;uniqueIndex" json:"name"`
	DisplayName     string    `gorm:"column:display_name" json:"display_name"`
	URL             string    `gorm:"column:url" json:"url"`
	CredentialsJSON JSON      `gorm:"column:credentials_json;type:jsonb" json:"-"`
	CreatedBy       *int64    `gorm:"column:created_by" json:"-"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Repository) TableName() string { return "helm_repositories" }

// RepositoryView is the API-safe projection of a Repository. Credentials
// are never included.
type RepositoryView struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	URL         string    `json:"url"`
	HasAuth     bool      `json:"has_auth"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateRepositoryRequest is the DTO for creating a new Helm repository.
type CreateRepositoryRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	URL         string `json:"url"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
}

// ChartSummary is a lightweight chart entry from the repo index.
type ChartSummary struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Home        string `json:"home,omitempty"`
	AppVersion  string `json:"app_version,omitempty"`
}

// ChartDetail includes all versions of a chart from the repo index.
type ChartDetail struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Icon        string         `json:"icon,omitempty"`
	Home        string         `json:"home,omitempty"`
	Maintainers []string       `json:"maintainers,omitempty"`
	Versions    []ChartVersion `json:"versions"`
}

// ChartVersion is a single version entry from the repo index.
type ChartVersion struct {
	Version    string `json:"version"`
	AppVersion string `json:"app_version,omitempty"`
	Created    string `json:"created,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

// DeployPreviewRequest is the internal DTO for chart deployment preview.
type DeployPreviewRequest struct {
	RepoID          int64
	ChartName       string
	ChartVersion    string
	TargetClusterID int64
	TargetNamespace string
	ReleaseName     string
	ValuesYAML      string
}

// Plan is the M19 controlled-operation plan for chart deployment.
// Mirrors promotion.Plan: confirmation token hash + idempotency key +
// Claim state machine. The ConfirmationToken field is gorm:"-" (never
// persisted) and is populated only on the Preview response.
type Plan struct {
	ID                    string     `gorm:"primaryKey;size:36" json:"id"`
	Status                string     `gorm:"column:status" json:"status"`
	RepoID                int64      `gorm:"column:repo_id" json:"repo_id"`
	ChartName             string     `gorm:"column:chart_name" json:"chart_name"`
	ChartVersion          string     `gorm:"column:chart_version" json:"chart_version"`
	TargetClusterID       int64      `gorm:"column:target_cluster_id" json:"target_cluster_id"`
	TargetNamespace       string     `gorm:"column:target_namespace" json:"target_namespace"`
	ReleaseName           string     `gorm:"column:release_name" json:"release_name"`
	ValuesYAML            string     `gorm:"column:values_yaml" json:"values_yaml,omitempty"`
	ChartMetadata         JSON       `gorm:"column:chart_metadata;type:jsonb" json:"chart_metadata"`
	ReleaseManifest       JSON       `gorm:"column:release_manifest;type:jsonb" json:"-"`
	DeployDiff            JSON       `gorm:"column:deploy_diff;type:jsonb" json:"deploy_diff"`
	ConfirmationTokenHash []byte     `gorm:"column:confirmation_token_hash" json:"-"`
	RequestedByUserID     *int64     `gorm:"column:requested_by_user_id" json:"-"`
	RequestedByName       string     `gorm:"column:requested_by_name" json:"-"`
	ExpiresAt             time.Time  `gorm:"column:expires_at" json:"expires_at"`
	IdempotencyKey        string     `gorm:"column:idempotency_key" json:"-"`
	LockedAt              *time.Time `gorm:"column:locked_at" json:"-"`
	ExecutedAt            *time.Time `gorm:"column:executed_at" json:"executed_at,omitempty"`
	LastError             string     `gorm:"column:last_error" json:"last_error,omitempty"`
	CreatedAt             time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at" json:"updated_at"`

	// ConfirmationToken is set transiently on the Preview response; never
	// persisted (gorm:"-") and never serialised in subsequent reads.
	ConfirmationToken string `gorm:"-" json:"confirmation_token,omitempty"`
}

func (Plan) TableName() string { return "app_catalog_plans" }

// PlanView is the API-safe projection of a Plan. The ReleaseManifest
// (the HelmRelease CR JSON) is excluded from list responses; the
// confirmation token is included only on the Preview response.
type PlanView struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	RepoID          int64      `json:"repo_id"`
	ChartName       string     `json:"chart_name"`
	ChartVersion    string     `json:"chart_version"`
	TargetClusterID int64      `json:"target_cluster_id"`
	TargetNamespace string     `json:"target_namespace"`
	ReleaseName     string     `json:"release_name"`
	DeployDiff      JSON       `json:"deploy_diff"`
	ExpiresAt       time.Time  `json:"expires_at"`
	ExecutedAt      *time.Time `json:"executed_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	// ConfirmationToken is populated only on the Preview response (201).
	ConfirmationToken string `json:"confirmation_token,omitempty"`
}

// DeployDiff is the typed preview diff shown to the operator before
// confirming a chart deployment. Mirrors promotion.ItemDiff.
type DeployDiff struct {
	Mode         string         `json:"mode"` // "create"
	ChartName    string         `json:"chart_name"`
	ChartVersion string         `json:"chart_version"`
	Namespace    string         `json:"namespace"`
	ReleaseName  string         `json:"release_name"`
	Values       map[string]any `json:"values,omitempty"`
}

// JSON is a helper type for JSONB columns, mirroring promotion.JSON.
type JSON json.RawMessage

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("{}"), nil
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	*j = append((*j)[:0], data...)
	return nil
}

// helmIndex is the minimal Helm repo index.yaml structure we parse.
type helmIndex struct {
	APIVersion string                      `yaml:"apiVersion"`
	Entries    map[string][]helmIndexEntry `yaml:"entries"`
}

// helmIndexEntry is a single version entry within a chart's entries list.
type helmIndexEntry struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Icon        string `yaml:"icon"`
	Home        string `yaml:"home"`
	AppVersion  string `yaml:"appVersion"`
	Digest      string `yaml:"digest"`
	Created     string `yaml:"created"`
	Maintainers []struct {
		Name  string `yaml:"name"`
		Email string `yaml:"email"`
	} `yaml:"maintainers"`
}
