package audit

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

type exportRepositoryStub struct{ response ListResponse }

func (s exportRepositoryStub) Save(context.Context, *Entry) error { return nil }
func (s exportRepositoryStub) List(context.Context, Filter) (ListResponse, error) {
	return s.response, nil
}

func TestExportCSVHasStableColumnsAndNeutralizesFormulas(t *testing.T) {
	clusterID := int64(7)
	repository := exportRepositoryStub{response: ListResponse{Total: 2, Remaining: 1, Items: []Entry{{
		ID: 9, Actor: Actor{ID: 3, Name: "=cmd|' /C calc'!A0"}, ClusterID: &clusterID, Action: "cluster.probe",
		Resource: ResourceRef{Type: "Cluster", Name: "+SUM(1,1)"}, Result: "failure", RequestID: "request-1", StatusCode: 400,
		IPAddress: "127.0.0.1", UserAgent: " @malicious", Details: map[string]any{"cluster_id": float64(7)}, CreatedAt: time.Date(2026, 7, 17, 4, 0, 0, 0, time.UTC),
	}}}}
	var buffer bytes.Buffer
	result, err := NewService(repository).ExportCSV(context.Background(), Filter{Limit: 1}, &buffer)
	if err != nil {
		t.Fatalf("ExportCSV() error = %v", err)
	}
	if result.Rows != 1 || result.Total != 2 || !result.Truncated {
		t.Fatalf("ExportCSV() result = %#v", result)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(buffer.String(), "\uFEFF")))
	records, err := reader.ReadAll()
	if err != nil || len(records) != 2 || len(records[0]) != 15 {
		t.Fatalf("CSV records=%#v err=%v", records, err)
	}
	if records[1][3][0] != '\'' || records[1][8][0] != '\'' || records[1][13][0] != '\'' {
		t.Fatalf("formula-like cells were not neutralized: %#v", records[1])
	}
}

func TestSafeCSVCellLeavesOrdinaryValuesUnchanged(t *testing.T) {
	if got := safeCSVCell("System Administrator"); got != "System Administrator" {
		t.Fatalf("safeCSVCell() = %q", got)
	}
}
