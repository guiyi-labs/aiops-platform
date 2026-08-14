package incident

import (
	"fmt"
	"io"
	"strings"
	"time"
)

func writeIncidentMarkdown(destination io.Writer, record Incident, evidence []EvidenceItem) error {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s %s\n\n", markdownInline(record.Number), markdownInline(record.Title))
	builder.WriteString("## Incident\n\n")
	fmt.Fprintf(&builder, "- Severity: %s\n", markdownInline(record.Severity))
	fmt.Fprintf(&builder, "- Status: %s\n", markdownInline(record.Status))
	fmt.Fprintf(&builder, "- Cluster: %d\n", record.ClusterID)
	fmt.Fprintf(&builder, "- Resource: %s\n", markdownInline(resourceLabel(record.Resource)))
	fmt.Fprintf(&builder, "- Source: %s (%s)\n", markdownInline(record.SourceType), markdownInline(record.SourceRef))
	if record.TemplateID != "" {
		fmt.Fprintf(&builder, "- Response template: %s\n", markdownInline(record.TemplateID))
	}
	fmt.Fprintf(&builder, "- Observed at: %s\n", formatIncidentTime(record.ObservedAt))
	fmt.Fprintf(&builder, "- SLA due at: %s\n", formatIncidentTime(record.SLADueAt))
	fmt.Fprintf(&builder, "- Resolved at: %s\n", formatOptionalIncidentTime(record.ResolvedAt))

	builder.WriteString("\n## Narrative\n\n")
	narrative := strings.TrimSpace(record.Postmortem)
	if narrative == "" {
		narrative = strings.TrimSpace(record.Summary)
	}
	writeMarkdownBlock(&builder, narrative)

	builder.WriteString("\n## Evidence timeline\n\n")
	if len(evidence) == 0 {
		builder.WriteString("No evidence was available.\n")
	} else {
		for _, item := range evidence {
			fmt.Fprintf(&builder, "- **%s** %s (%s)\n", markdownInline(item.SourceType), markdownInline(item.Title), markdownInline(item.SourceRef))
			if item.Summary != "" {
				fmt.Fprintf(&builder, "  - Summary: %s\n", markdownInline(item.Summary))
			}
			if item.Resource.Name != "" {
				fmt.Fprintf(&builder, "  - Resource: %s\n", markdownInline(resourceLabel(item.Resource)))
			}
			if item.ObservedAt != "" {
				fmt.Fprintf(&builder, "  - Observed at: %s\n", markdownInline(item.ObservedAt))
			}
			if item.DeepLink != "" {
				fmt.Fprintf(&builder, "  - Source link: `%s`\n", markdownInline(item.DeepLink))
			}
		}
	}

	builder.WriteString("\n## Decisions and actions\n\n")
	if len(record.Timeline) == 0 {
		builder.WriteString("No timeline events were recorded.\n")
	} else {
		for _, event := range record.Timeline {
			kind := "Decision"
			if event.EventType == EventTypeNote {
				kind = "Operator note"
			}
			fmt.Fprintf(&builder, "- **%s** %s (%s, %s): %s\n", kind, markdownInline(event.Actor.Name), formatIncidentTime(event.CreatedAt), event.EventType, markdownInline(event.Content))
		}
	}

	builder.WriteString("\n## Outcome\n\n")
	if record.ResolvedAt == nil {
		builder.WriteString("- Resolution: unresolved\n")
	} else {
		resolution := record.ResolvedAt.Sub(record.ObservedAt)
		if resolution < 0 {
			resolution = 0
		}
		fmt.Fprintf(&builder, "- Resolution: resolved in %s\n", resolution.Round(time.Second))
		result := "SLA met"
		if record.ResolvedAt.After(record.SLADueAt) {
			result = "SLA missed"
		}
		fmt.Fprintf(&builder, "- SLA result: %s\n", result)
	}
	fmt.Fprintf(&builder, "- Timeline events: %d\n", len(record.Timeline))
	fmt.Fprintf(&builder, "- Evidence sources: %d\n", len(evidence))

	_, err := io.WriteString(destination, builder.String())
	return err
}

func resourceLabel(resource ResourceRef) string {
	if resource.Namespace == "" {
		return resource.Kind + "/" + resource.Name
	}
	return resource.Kind + "/" + resource.Namespace + "/" + resource.Name
}

func markdownInline(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func writeMarkdownBlock(builder *strings.Builder, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		builder.WriteString("No narrative was recorded.\n")
		return
	}
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		builder.WriteString("> ")
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
}

func formatIncidentTime(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.UTC().Format(time.RFC3339)
}

func formatOptionalIncidentTime(value *time.Time) string {
	if value == nil {
		return "not resolved"
	}
	return formatIncidentTime(*value)
}
