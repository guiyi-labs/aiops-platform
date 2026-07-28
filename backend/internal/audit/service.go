package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
)

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) Record(ctx context.Context, entry *Entry) error {
	return s.repository.Save(ctx, entry)
}

func (s *Service) List(ctx context.Context, filter Filter) (ListResponse, error) {
	return s.repository.List(ctx, filter)
}

func (s *Service) ExportCSV(ctx context.Context, filter Filter, destination io.Writer) (ExportResult, error) {
	response, err := s.repository.List(ctx, filter)
	if err != nil {
		return ExportResult{}, err
	}
	if _, err := io.WriteString(destination, "\uFEFF"); err != nil {
		return ExportResult{}, err
	}
	writer := csv.NewWriter(destination)
	writer.UseCRLF = true
	if err := writer.Write([]string{"id", "created_at", "actor_id", "actor_name", "cluster_id", "action", "resource_type", "resource_namespace", "resource_name", "result", "request_id", "status_code", "ip_address", "user_agent", "details_json"}); err != nil {
		return ExportResult{}, err
	}
	for _, entry := range response.Items {
		details, err := json.Marshal(entry.Details)
		if err != nil {
			return ExportResult{}, err
		}
		clusterID := ""
		if entry.ClusterID != nil {
			clusterID = strconv.FormatInt(*entry.ClusterID, 10)
		}
		row := []string{
			strconv.FormatInt(entry.ID, 10), entry.CreatedAt.UTC().Format(time.RFC3339Nano), strconv.FormatInt(entry.Actor.ID, 10), safeCSVCell(entry.Actor.Name), clusterID,
			safeCSVCell(entry.Action), safeCSVCell(entry.Resource.Type), safeCSVCell(entry.Resource.Namespace), safeCSVCell(entry.Resource.Name), safeCSVCell(entry.Result),
			safeCSVCell(entry.RequestID), strconv.Itoa(entry.StatusCode), safeCSVCell(entry.IPAddress), safeCSVCell(entry.UserAgent), safeCSVCell(string(details)),
		}
		if err := writer.Write(row); err != nil {
			return ExportResult{}, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Rows: len(response.Items), Total: response.Total, Truncated: response.Remaining > 0}, nil
}

func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
