package authz

import "testing"

func TestModelTableNames(t *testing.T) {
	if (ClusterGrant{}).TableName() != "user_cluster_grants" {
		t.Errorf("ClusterGrant.TableName = %q", (ClusterGrant{}).TableName())
	}
	if (NamespaceGrant{}).TableName() != "user_namespace_grants" {
		t.Errorf("NamespaceGrant.TableName = %q", (NamespaceGrant{}).TableName())
	}
}
