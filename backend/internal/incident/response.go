package incident

import (
	"sort"
	"strings"
	"time"
)

type SeverityTarget struct {
	Severity      string `json:"severity"`
	TargetMinutes int    `json:"target_minutes"`
}

type ResponseTemplate struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	SourceTypes     []string `json:"source_types"`
	DefaultTitle    string   `json:"default_title"`
	DefaultSeverity string   `json:"default_severity"`
	DefaultSummary  string   `json:"default_summary"`
	Steps           []string `json:"steps"`
}

type ResponseCatalog struct {
	Templates      []ResponseTemplate `json:"templates"`
	SeverityMatrix []SeverityTarget   `json:"severity_matrix"`
}

func DefaultResponseCatalog() ResponseCatalog {
	return ResponseCatalog{
		Templates: []ResponseTemplate{
			{
				ID: "generic", Name: "通用事故", Description: "适用于人工上报或无法匹配专用流程的事故。",
				SourceTypes: allSourceTypes(), DefaultTitle: "待确认的运营事故", DefaultSeverity: SeverityWarning,
				DefaultSummary: "记录影响范围、当前症状与初步判断。",
				Steps:          []string{"确认影响范围", "指定负责人", "记录处理决策", "验证恢复并关闭"},
			},
			{
				ID: "node-not-ready", Name: "Node NotReady", Description: "节点不可调度或 kubelet 不可用时的响应起点。",
				SourceTypes: []string{SourceTypeDiagnosis, SourceTypeFinding}, DefaultTitle: "节点不可用事故", DefaultSeverity: SeverityCritical,
				DefaultSummary: "确认节点状态、工作负载影响和可用替代节点。",
				Steps:          []string{"确认节点与 kubelet 状态", "评估受影响工作负载", "执行受控迁移或恢复", "验证节点恢复与业务健康"},
			},
			{
				ID: "deployment-unavailable", Name: "Deployment Unavailable", Description: "Deployment 可用副本不足或发布不可用时的响应起点。",
				SourceTypes: []string{SourceTypeDiagnosis, SourceTypeFinding}, DefaultTitle: "Deployment 不可用事故", DefaultSeverity: SeverityHigh,
				DefaultSummary: "确认副本、发布变更和就绪探针状态。",
				Steps:          []string{"确认副本与就绪探针", "检查最近发布变更", "先预览再执行受控回滚或重启", "验证可用副本恢复"},
			},
			{
				ID: "oom-killed", Name: "OOMKilled", Description: "容器因内存压力退出或反复重启时的响应起点。",
				SourceTypes: []string{SourceTypeDiagnosis, SourceTypeFinding}, DefaultTitle: "容器内存耗尽事故", DefaultSeverity: SeverityHigh,
				DefaultSummary: "确认容器退出证据、资源限制和业务影响。",
				Steps:          []string{"确认退出原因与影响容器", "对比工作负载内存曲线", "评估限制调整或受控重启", "验证重启后稳定性"},
			},
		},
		SeverityMatrix: []SeverityTarget{
			{Severity: SeverityCritical, TargetMinutes: 60},
			{Severity: SeverityHigh, TargetMinutes: 240},
			{Severity: SeverityWarning, TargetMinutes: 1440},
			{Severity: SeverityInfo, TargetMinutes: 4320},
		},
	}
}

func allSourceTypes() []string {
	return []string{SourceTypeDiagnosis, SourceTypeFinding, SourceTypeAlert, SourceTypeInspection, SourceTypeSignal, SourceTypeCorrelation}
}

func (c ResponseCatalog) WithSLADurations(durations map[string]time.Duration) ResponseCatalog {
	if len(durations) == 0 {
		return c
	}
	matrix := make([]SeverityTarget, 0, len(c.SeverityMatrix))
	for _, target := range c.SeverityMatrix {
		if duration, ok := durations[target.Severity]; ok && duration > 0 {
			target.TargetMinutes = int(duration / time.Minute)
			if target.TargetMinutes < 1 {
				target.TargetMinutes = 1
			}
		}
		matrix = append(matrix, target)
	}
	c.SeverityMatrix = matrix
	return c
}

func (c ResponseCatalog) Template(id string) (ResponseTemplate, bool) {
	id = strings.TrimSpace(id)
	for _, template := range c.Templates {
		if template.ID == id {
			return cloneTemplate(template), true
		}
	}
	return ResponseTemplate{}, false
}

func (c ResponseCatalog) SLADeadline(severity string, observedAt time.Time) time.Time {
	for _, target := range c.SeverityMatrix {
		if target.Severity == severity && target.TargetMinutes > 0 {
			return observedAt.UTC().Add(time.Duration(target.TargetMinutes) * time.Minute)
		}
	}
	return SLADeadline(severity, observedAt)
}

func (t ResponseTemplate) SupportsSource(sourceType string) bool {
	for _, supported := range t.SourceTypes {
		if supported == sourceType {
			return true
		}
	}
	return false
}

func cloneTemplate(template ResponseTemplate) ResponseTemplate {
	template.SourceTypes = append([]string(nil), template.SourceTypes...)
	template.Steps = append([]string(nil), template.Steps...)
	return template
}

func (c ResponseCatalog) clone() ResponseCatalog {
	cloned := ResponseCatalog{SeverityMatrix: append([]SeverityTarget(nil), c.SeverityMatrix...)}
	cloned.Templates = make([]ResponseTemplate, 0, len(c.Templates))
	for _, template := range c.Templates {
		cloned.Templates = append(cloned.Templates, cloneTemplate(template))
	}
	sort.SliceStable(cloned.Templates, func(i, j int) bool { return cloned.Templates[i].ID < cloned.Templates[j].ID })
	return cloned
}
