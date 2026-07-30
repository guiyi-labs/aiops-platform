package remediation

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestPlanRollbackFieldsMatchMigrationColumns(t *testing.T) {
	parsed, err := schema.Parse(&Plan{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	for fieldName, wantColumn := range map[string]string{
		"RollbackReplicaSetName":            "rollback_replicaset_name",
		"RollbackReplicaSetUID":             "rollback_replicaset_uid",
		"RollbackReplicaSetResourceVersion": "rollback_replicaset_resource_version",
	} {
		field := parsed.LookUpField(fieldName)
		gotColumn := "<missing>"
		if field != nil {
			gotColumn = field.DBName
		}
		if gotColumn != wantColumn {
			t.Errorf("%s column = %q, want %q", fieldName, gotColumn, wantColumn)
		}
	}
}
