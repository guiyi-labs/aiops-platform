package httpserver

import (
	"fmt"
	"sort"
	"strings"
)

// RouteScope classifies the resource-scope dimension of a route. The scope is
// derived from the route's path template: namespace-scoped routes are also
// cluster-scoped (group nesting), workspace routes are keyed by workspace_id,
// and platform-global routes carry no resource key.
type RouteScope string

const (
	ScopeNone      RouteScope = "none"      // platform-global, no resource key
	ScopeWorkspace RouteScope = "workspace" // keyed by workspace_id
	ScopeCluster   RouteScope = "cluster"   // keyed by cluster_id
	ScopeNamespace RouteScope = "namespace" // keyed by cluster_id + namespace
)

// PermissionEntry is one row of the generated permission matrix.
type PermissionEntry struct {
	Method      string
	Path        string
	Roles       []string // sorted, deduplicated; empty means any authenticated user
	Scope       RouteScope
	AuditAction string
}

// PermissionMatrix is the full route -> (roles, scope, audit) mapping derived
// from the RouteDescriptor registry. It is the single source of truth used by
// the committed permission-matrix document and its diff gate.
type PermissionMatrix struct {
	Version string
	Entries []PermissionEntry
}

// MatrixVersion is the contract version of the generated matrix document.
const MatrixVersion = "1.0"

// BuildPermissionMatrix derives the matrix from the current routeTable. The
// table is populated during New(); callers must have built the router first.
// Entries are sorted by (path, method) for deterministic rendering.
func BuildPermissionMatrix() PermissionMatrix {
	entries := make([]PermissionEntry, 0, len(routeTable))
	for _, r := range routeTable {
		entries = append(entries, PermissionEntry{
			Method:      r.Method,
			Path:        r.FullPath,
			Roles:       append([]string(nil), r.RequiredRoles...),
			Scope:       classifyScope(r.FullPath),
			AuditAction: r.AuditAction,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Method < entries[j].Method
	})
	for i := range entries {
		sort.Strings(entries[i].Roles)
	}
	return PermissionMatrix{Version: MatrixVersion, Entries: entries}
}

// classifyScope derives the resource scope from the path template. Namespace
// paths always carry :cluster_id as well (group nesting), so namespace is
// checked before cluster.
func classifyScope(path string) RouteScope {
	switch {
	case strings.Contains(path, ":namespace"):
		return ScopeNamespace
	case strings.Contains(path, ":cluster_id"):
		return ScopeCluster
	case strings.Contains(path, ":workspace_id"):
		return ScopeWorkspace
	default:
		return ScopeNone
	}
}

// RenderMarkdown renders the matrix as a deterministic document. It must not
// embed timestamps: the output is committed and diff-gated by tests.
func (m PermissionMatrix) RenderMarkdown() string {
	var b strings.Builder
	b.WriteString("# 路由权限矩阵（Permission Matrix）\n\n")
	b.WriteString("> 由 `backend/internal/httpserver` 的 RouteDescriptor 注册表自动生成，\n")
	b.WriteString("> `TestPermissionMatrixMatchesCommittedDocument` 负责差异门禁：任何路由、角色或\n")
	b.WriteString("> 审计元数据变更必须同步更新本文档（`go test ./internal/httpserver -run TestPermissionMatrixMatchesCommittedDocument -update`）。\n\n")
	fmt.Fprintf(&b, "- 版本：%s\n", m.Version)
	b.WriteString("- 角色集合：`system_admin` / `operations_admin` / `security_auditor` / `viewer`\n")
	b.WriteString("- 资源维度（scope）：`workspace` / `cluster` / `namespace` / `none`（平台级）\n")
	b.WriteString("- 角色为空表示任意已认证用户可访问；审计动作为空表示不审计\n\n")
	b.WriteString("> 说明：路径维度无法体现查询参数。以 `?cluster_id=` / `?namespace=` 驱动的集群/命名空间访问\n")
	b.WriteString("> （如 `/aiops` 列表路由）由 `requireClusterQueryAccess` 中间件在运行时校验（M100），\n")
	b.WriteString("> 表中此类路由标记为 `none`。\n\n")

	byScope := map[RouteScope]int{}
	restricted := 0
	audited := 0
	byRole := map[string]int{}
	for _, e := range m.Entries {
		byScope[e.Scope]++
		if len(e.Roles) > 0 {
			restricted++
		}
		if e.AuditAction != "" {
			audited++
		}
		for _, role := range e.Roles {
			byRole[role]++
		}
	}

	b.WriteString("## 汇总\n\n")
	b.WriteString("| 维度 | 值 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 路由总数 | %d |\n", len(m.Entries))
	fmt.Fprintf(&b, "| 角色受限 | %d |\n", restricted)
	fmt.Fprintf(&b, "| 已审计 | %d |\n", audited)
	for _, scope := range []RouteScope{ScopeWorkspace, ScopeCluster, ScopeNamespace, ScopeNone} {
		fmt.Fprintf(&b, "| scope=%s | %d |\n", scope, byScope[scope])
	}
	roleNames := make([]string, 0, len(byRole))
	for role := range byRole {
		roleNames = append(roleNames, role)
	}
	sort.Strings(roleNames)
	for _, role := range roleNames {
		fmt.Fprintf(&b, "| 角色 %s | %d |\n", role, byRole[role])
	}

	b.WriteString("\n## 路由明细（按路径排序）\n\n")
	b.WriteString("| 方法 | 路径 | 角色 | 维度 | 审计动作 |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, e := range m.Entries {
		roles := "any"
		if len(e.Roles) > 0 {
			roles = "`" + strings.Join(e.Roles, "`, `") + "`"
		}
		audit := e.AuditAction
		if audit == "" {
			audit = "-"
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s |\n", e.Method, e.Path, roles, e.Scope, audit)
	}
	return b.String()
}
