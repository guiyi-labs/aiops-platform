package recoveryreadiness

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"
)

const (
	PolicyFormat   = "aiops.recovery-readiness-policy/v1"
	EvidenceFormat = "aiops.logical-restore-evidence/v1"
	ReportFormat   = "aiops.recovery-readiness-report/v1"
	maxFileSize    = 1 << 20
)

type Policy struct {
	Format                       string         `json:"format"`
	SystemName                   string         `json:"system_name"`
	DataClassification           string         `json:"data_classification"`
	ProductionValidationRequired bool           `json:"production_validation_required"`
	Owners                       Owners         `json:"owners"`
	Objectives                   Objectives     `json:"objectives"`
	Storage                      StoragePolicy  `json:"storage"`
	Backup                       BackupPolicy   `json:"backup"`
	PITR                         PITRPolicy     `json:"pitr"`
	HA                           HAPolicy       `json:"ha"`
	Drills                       DrillPolicy    `json:"drills"`
	Cutover                      CutoverPolicy  `json:"cutover"`
	Approvals                    ApprovalPolicy `json:"approvals"`
}

type Owners struct {
	ServiceOwner      string `json:"service_owner"`
	DatabaseOwner     string `json:"database_owner"`
	SecurityOwner     string `json:"security_owner"`
	IncidentCommander string `json:"incident_commander"`
}

type Objectives struct {
	RPOMinutes                      int `json:"rpo_minutes"`
	RTOMinutes                      int `json:"rto_minutes"`
	MaximumTolerableDowntimeMinutes int `json:"maximum_tolerable_downtime_minutes"`
}

type StoragePolicy struct {
	OffCluster        bool `json:"off_cluster"`
	Encrypted         bool `json:"encrypted"`
	RetentionDays     int  `json:"retention_days"`
	ImmutableDays     int  `json:"immutable_days"`
	IndependentCopies int  `json:"independent_copies"`
	Regions           int  `json:"regions"`
}

type BackupPolicy struct {
	FullIntervalHours      int  `json:"full_interval_hours"`
	VerifyAfterBackup      bool `json:"verify_after_backup"`
	LeastPrivilegeIdentity bool `json:"least_privilege_identity"`
	CredentialRotationDays int  `json:"credential_rotation_days"`
}

type PITRPolicy struct {
	Enabled                   bool   `json:"enabled"`
	WALArchiveIntervalSeconds int    `json:"wal_archive_interval_seconds"`
	RecoveryWindowHours       int    `json:"recovery_window_hours"`
	Encrypted                 bool   `json:"encrypted"`
	GapMonitoring             bool   `json:"gap_monitoring"`
	RestoreTargetIsolation    bool   `json:"restore_target_isolation"`
	RiskAcceptanceOwner       string `json:"risk_acceptance_owner"`
	RiskAcceptanceExpiresAt   string `json:"risk_acceptance_expires_at"`
}

type HAPolicy struct {
	Enabled                 bool   `json:"enabled"`
	Topology                string `json:"topology"`
	DatabaseReplicas        int    `json:"database_replicas"`
	AutomaticFailover       bool   `json:"automatic_failover"`
	FailoverOwner           string `json:"failover_owner"`
	WriterFencing           bool   `json:"writer_fencing"`
	MaxFailoverMinutes      int    `json:"max_failover_minutes"`
	RiskAcceptanceOwner     string `json:"risk_acceptance_owner"`
	RiskAcceptanceExpiresAt string `json:"risk_acceptance_expires_at"`
}

type DrillPolicy struct {
	LogicalRestoreIntervalDays   int  `json:"logical_restore_interval_days"`
	PITRIntervalDays             int  `json:"pitr_interval_days"`
	FailoverIntervalDays         int  `json:"failover_interval_days"`
	ProductionRepresentativeData bool `json:"production_representative_data"`
	IsolatedTarget               bool `json:"isolated_target"`
}

type CutoverPolicy struct {
	WriterFreeze    bool   `json:"writer_freeze"`
	TrafficOwner    string `json:"traffic_owner"`
	ValidationOwner string `json:"validation_owner"`
	RollbackPlan    string `json:"rollback_plan"`
}

type ApprovalPolicy struct {
	ServiceOwner  string `json:"service_owner"`
	DatabaseOwner string `json:"database_owner"`
	SecurityOwner string `json:"security_owner"`
}

type LogicalRestoreEvidence struct {
	Format                       string           `json:"format"`
	VerifiedAt                   string           `json:"verified_at"`
	PostgresImage                string           `json:"postgres_image"`
	DatabaseName                 string           `json:"database_name"`
	MigrationCount               int              `json:"migration_count"`
	LatestMigration              string           `json:"latest_migration"`
	Backup                       BackupEvidence   `json:"backup"`
	SourceDestroyedBeforeRestore bool             `json:"source_destroyed_before_restore"`
	SourceSnapshot               map[string]int64 `json:"source_snapshot"`
	RestoredSnapshot             map[string]int64 `json:"restored_snapshot"`
	Cleanup                      CleanupEvidence  `json:"cleanup"`
}

type BackupEvidence struct {
	Format   string `json:"format"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Retained bool   `json:"retained"`
}

type CleanupEvidence struct {
	SourceContainerDeleted     bool `json:"source_container_deleted"`
	TargetContainerDeleted     bool `json:"target_container_deleted"`
	TemporaryFilesDeleted      bool `json:"temporary_files_deleted"`
	ProcessEnvironmentRestored bool `json:"process_environment_restored"`
}

type Check struct {
	Code   string `json:"code"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type Report struct {
	Format                       string  `json:"format"`
	ReadyForPITRHAImplementation bool    `json:"ready_for_pitr_ha_implementation"`
	ProductionRecoveryValidated  bool    `json:"production_recovery_validated"`
	SystemName                   string  `json:"system_name"`
	Passed                       int     `json:"passed"`
	Failed                       int     `json:"failed"`
	Checks                       []Check `json:"checks"`
}

func LoadPolicy(path string) (Policy, error) {
	var value Policy
	return value, decodeStrictFile(path, &value)
}

func LoadEvidence(path string) (LogicalRestoreEvidence, error) {
	var value LogicalRestoreEvidence
	return value, decodeStrictFile(path, &value)
}

func Evaluate(policy Policy, evidence LogicalRestoreEvidence, now time.Time) Report {
	report := Report{Format: ReportFormat, SystemName: policy.SystemName, ProductionRecoveryValidated: false}
	add := func(code string, passed bool, success, failure string) {
		detail := failure
		if passed {
			detail = success
			report.Passed++
		} else {
			report.Failed++
		}
		report.Checks = append(report.Checks, Check{Code: code, Passed: passed, Detail: detail})
	}

	add("policy.format", policy.Format == PolicyFormat && evidence.Format == EvidenceFormat, "policy and logical-restore evidence formats are supported", "policy or evidence format is unsupported")
	add("policy.owners", allNonEmpty(policy.SystemName, policy.DataClassification, policy.Owners.ServiceOwner, policy.Owners.DatabaseOwner, policy.Owners.SecurityOwner, policy.Owners.IncidentCommander), "system classification and operational owners are assigned", "system name, classification and all operational owners are required")
	objectivesOK := policy.Objectives.RPOMinutes >= 1 && policy.Objectives.RPOMinutes <= 1440 && policy.Objectives.RTOMinutes >= 1 && policy.Objectives.RTOMinutes <= 2880 && policy.Objectives.MaximumTolerableDowntimeMinutes >= policy.Objectives.RTOMinutes
	add("recovery.objectives", objectivesOK, "bounded RPO, RTO and maximum tolerable downtime are explicit", "RPO must be 1..1440 minutes, RTO 1..2880 and maximum tolerable downtime must cover RTO")
	storageOK := policy.Storage.OffCluster && policy.Storage.Encrypted && policy.Storage.RetentionDays >= 7 && policy.Storage.RetentionDays <= 3650 && policy.Storage.ImmutableDays >= 1 && policy.Storage.ImmutableDays <= policy.Storage.RetentionDays && policy.Storage.IndependentCopies >= 2 && policy.Storage.IndependentCopies <= 10 && policy.Storage.Regions >= 1 && policy.Storage.Regions <= 5
	add("recovery.storage", storageOK, "encrypted off-cluster retention, immutability and independent copies are defined", "storage requires encryption, off-cluster placement, 7..3650 day retention, bounded immutability and at least two copies")
	backupOK := policy.Backup.FullIntervalHours >= 1 && policy.Backup.FullIntervalHours <= 168 && policy.Backup.VerifyAfterBackup && policy.Backup.LeastPrivilegeIdentity && policy.Backup.CredentialRotationDays >= 1 && policy.Backup.CredentialRotationDays <= 365
	if !policy.PITR.Enabled {
		backupOK = backupOK && policy.Backup.FullIntervalHours*60 <= policy.Objectives.RPOMinutes
	}
	add("recovery.backup", backupOK, "full-backup schedule, verification and least-privilege identity satisfy the selected RPO path", "backup frequency must meet RPO without PITR and verification/least-privilege/rotation controls are required")
	pitrOK := evaluatePITR(policy.PITR, policy.Objectives.RPOMinutes, now)
	add("recovery.pitr", pitrOK, "PITR controls or a current explicit risk acceptance are complete", "PITR requires bounded WAL archival, recovery window, encryption, gap monitoring and isolation, or a current risk acceptance")
	haOK := evaluateHA(policy.HA, policy.Objectives.RTOMinutes, now)
	add("recovery.ha", haOK, "HA failover controls or a current explicit risk acceptance are complete", "HA requires a supported topology, replicas, owner, fencing and RTO-bound failover, or a current risk acceptance")
	drillsOK := policy.Drills.LogicalRestoreIntervalDays >= 1 && policy.Drills.LogicalRestoreIntervalDays <= 90 && policy.Drills.ProductionRepresentativeData && policy.Drills.IsolatedTarget
	if policy.PITR.Enabled {
		drillsOK = drillsOK && policy.Drills.PITRIntervalDays >= 1 && policy.Drills.PITRIntervalDays <= 90
	}
	if policy.HA.Enabled {
		drillsOK = drillsOK && policy.Drills.FailoverIntervalDays >= 1 && policy.Drills.FailoverIntervalDays <= 180
	}
	add("recovery.drills", drillsOK, "isolated representative restore and enabled-capability drill intervals are bounded", "logical restore must run within 90 days and enabled PITR/HA capabilities need bounded drills")
	cutoverOK := policy.Cutover.WriterFreeze && allNonEmpty(policy.Cutover.TrafficOwner, policy.Cutover.ValidationOwner, policy.Cutover.RollbackPlan)
	add("recovery.cutover", cutoverOK, "writer fencing, traffic/validation ownership and rollback are explicit", "cutover requires writer freeze, accountable traffic/validation owners and a rollback plan")
	approvalsOK := allNonEmpty(policy.Approvals.ServiceOwner, policy.Approvals.DatabaseOwner, policy.Approvals.SecurityOwner)
	add("policy.approvals", approvalsOK, "service, database and security approvals are recorded", "service, database and security approvals are required")
	verifiedAt, timeErr := time.Parse(time.RFC3339, evidence.VerifiedAt)
	freshnessOK := timeErr == nil && !verifiedAt.After(now.Add(5*time.Minute)) && now.Sub(verifiedAt) <= time.Duration(policy.Drills.LogicalRestoreIntervalDays)*24*time.Hour
	add("evidence.freshness", freshnessOK, "logical restore evidence is within the approved drill interval", "logical restore evidence timestamp is invalid, future-dated or stale")
	logicalOK := evidence.SourceDestroyedBeforeRestore && evidence.MigrationCount > 0 && allNonEmpty(evidence.PostgresImage, evidence.DatabaseName, evidence.LatestMigration) && evidence.Backup.Format == "custom" && evidence.Backup.Bytes > 0 && !evidence.Backup.Retained
	add("evidence.logical_restore", logicalOK, "fresh-target logical restore evidence is complete and the dump was not retained", "evidence must prove a non-empty custom backup, source destruction, migrations and no retained dump")
	digest, digestErr := hex.DecodeString(evidence.Backup.SHA256)
	integrityOK := digestErr == nil && len(digest) == 32 && len(evidence.SourceSnapshot) > 0 && reflect.DeepEqual(evidence.SourceSnapshot, evidence.RestoredSnapshot) && evidence.SourceSnapshot["invalid_foreign_keys"] == 0
	add("evidence.integrity", integrityOK, "backup digest, source/restore snapshots and foreign keys are consistent", "evidence requires a SHA-256 digest, equal non-empty snapshots and zero invalid foreign keys")
	cleanupOK := evidence.Cleanup.SourceContainerDeleted && evidence.Cleanup.TargetContainerDeleted && evidence.Cleanup.TemporaryFilesDeleted && evidence.Cleanup.ProcessEnvironmentRestored
	add("evidence.cleanup", cleanupOK, "logical drill containers, files and process environment were cleaned", "all logical-drill cleanup assertions must pass")
	add("production.boundary", policy.ProductionValidationRequired, "policy explicitly requires production-size PITR/HA validation before a readiness claim", "production_validation_required must remain true")

	report.ReadyForPITRHAImplementation = report.Failed == 0
	return report
}

func evaluatePITR(policy PITRPolicy, rpoMinutes int, now time.Time) bool {
	if policy.Enabled {
		return policy.WALArchiveIntervalSeconds >= 1 && policy.WALArchiveIntervalSeconds <= rpoMinutes*60 && policy.RecoveryWindowHours >= 24 && policy.RecoveryWindowHours <= 8760 && policy.Encrypted && policy.GapMonitoring && policy.RestoreTargetIsolation && policy.RiskAcceptanceOwner == "" && policy.RiskAcceptanceExpiresAt == ""
	}
	return validRiskAcceptance(policy.RiskAcceptanceOwner, policy.RiskAcceptanceExpiresAt, now)
}

func evaluateHA(policy HAPolicy, rtoMinutes int, now time.Time) bool {
	if policy.Enabled {
		return slices.Contains([]string{"single_region_multi_zone", "multi_region"}, policy.Topology) && policy.DatabaseReplicas >= 2 && policy.DatabaseReplicas <= 7 && policy.AutomaticFailover && allNonEmpty(policy.FailoverOwner) && policy.WriterFencing && policy.MaxFailoverMinutes >= 1 && policy.MaxFailoverMinutes <= rtoMinutes && policy.RiskAcceptanceOwner == "" && policy.RiskAcceptanceExpiresAt == ""
	}
	return validRiskAcceptance(policy.RiskAcceptanceOwner, policy.RiskAcceptanceExpiresAt, now)
}

func validRiskAcceptance(owner, expiresAt string, now time.Time) bool {
	expires, err := time.Parse(time.RFC3339, expiresAt)
	return strings.TrimSpace(owner) != "" && err == nil && expires.After(now) && expires.Sub(now) <= 180*24*time.Hour
}

func decodeStrictFile(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return err
	}
	if len(contents) == 0 || len(contents) > maxFileSize {
		return fmt.Errorf("file must contain 1..%d bytes", maxFileSize)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("file must contain one JSON value")
	}
	return nil
}

func allNonEmpty(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}
