package knowledge

import (
	"context"

	"k8s-aiops.local/backend/internal/diagnosis"
)

// DiagnosisIngester implements diagnosis.KnowledgeIngester by converting the
// distilled input into a knowledge Entry and persisting it. The dependency
// direction is knowledge → diagnosis only (diagnosis stays free of any
// knowledge import, preserving the narrow contract).
type DiagnosisIngester struct {
	repo Repository
}

// NewDiagnosisIngester builds the diagnosis→knowledge adapter.
func NewDiagnosisIngester(repo Repository) *DiagnosisIngester {
	return &DiagnosisIngester{repo: repo}
}

// IngestResolved converts and persists the input. It returns nil on success;
// errors are surfaced so the diagnosis caller can log them (it never fails
// the transition itself — the diagnosis hook swallows errors).
func (i *DiagnosisIngester) IngestResolved(ctx context.Context, input diagnosis.KnowledgeEntryInput) error {
	entry := Entry{
		SourceDiagnosisID: input.SourceDiagnosisID,
		RuleID:            input.RuleID,
		Severity:          input.Severity,
		ResourceKind:      input.ResourceKind,
		ResourceNamespace: input.ResourceNamespace,
		ResourceName:      input.ResourceName,
		Summary:           input.Summary,
		RootCauses:        input.RootCauses,
		Recommendations:   input.Recommendations,
		NotedAt:           input.NotedAt,
	}
	_, err := i.repo.Insert(ctx, entry)
	return err
}
