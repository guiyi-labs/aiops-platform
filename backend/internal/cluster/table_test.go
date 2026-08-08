package cluster

import (
	"regexp"
	"testing"
)

func TestModelTableNames(t *testing.T) {
	if (Cluster{}).TableName() != "clusters" {
		t.Errorf("Cluster.TableName = %q", (Cluster{}).TableName())
	}
	if (Credential{}).TableName() != "cluster_credentials" {
		t.Errorf("Credential.TableName = %q", (Credential{}).TableName())
	}
	if (Condition{}).TableName() != "cluster_conditions" {
		t.Errorf("Condition.TableName = %q", (Condition{}).TableName())
	}
}

func TestAPIStatusError(t *testing.T) {
	e := APIStatusError{StatusCode: 404}
	if e.Error() != "Kubernetes API returned status 404" {
		t.Errorf("APIStatusError.Error = %q", e.Error())
	}
	if e.StatusCode != 404 {
		t.Errorf("APIStatusError status = %d", e.StatusCode)
	}
}

func TestNewCredentialReencryptionID(t *testing.T) {
	id, err := newCredentialReencryptionID()
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := regexp.MatchString("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", id); !ok {
		t.Errorf("id = %q, want UUID v4 shape", id)
	}
}
