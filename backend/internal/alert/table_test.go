package alert

import "testing"

func TestModelTableNames(t *testing.T) {
	if (Rule{}).TableName() != "alert_rules" {
		t.Errorf("Rule.TableName = %q", (Rule{}).TableName())
	}
	if (Instance{}).TableName() != "alert_instances" {
		t.Errorf("Instance.TableName = %q", (Instance{}).TableName())
	}
}
