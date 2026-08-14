# 路由权限矩阵（Permission Matrix）

> 由 `backend/internal/httpserver` 的 RouteDescriptor 注册表自动生成，
> `TestPermissionMatrixMatchesCommittedDocument` 负责差异门禁：任何路由、角色或
> 审计元数据变更必须同步更新本文档（`go test ./internal/httpserver -run TestPermissionMatrixMatchesCommittedDocument -update`）。

- 版本：1.0
- 角色集合：`system_admin` / `operations_admin` / `security_auditor` / `viewer`
- 资源维度（scope）：`workspace` / `cluster` / `namespace` / `none`（平台级）
- 角色为空表示任意已认证用户可访问；审计动作为空表示不审计

> 说明：路径维度无法体现查询参数。以 `?cluster_id=` / `?namespace=` 驱动的集群/命名空间访问
> （如 `/aiops` 列表路由）由 `requireClusterQueryAccess` 中间件在运行时校验（M100），
> 表中此类路由标记为 `none`。

## 汇总

| 维度 | 值 |
|---|---|
| 路由总数 | 304 |
| 角色受限 | 87 |
| 已审计 | 180 |
| scope=workspace | 13 |
| scope=cluster | 84 |
| scope=namespace | 32 |
| scope=none | 175 |
| 角色 operations_admin | 66 |
| 角色 security_auditor | 4 |
| 角色 system_admin | 87 |

## 路由明细（按路径排序）

| 方法 | 路径 | 角色 | 维度 | 审计动作 |
|---|---|---|---|---|
| GET | `/api/v1/ai/coverage` | any | none | ai_explanation.coverage.read |
| POST | `/api/v1/ai/explanations/:explanation_id/feedback` | any | none | ai_explanation.feedback.create |
| GET | `/api/v1/ai/quality` | any | none | - |
| GET | `/api/v1/ai/status` | any | none | - |
| GET | `/api/v1/aiops/automation/plans` | any | none | aiops.automation.plans.list |
| POST | `/api/v1/aiops/automation/plans` | `operations_admin`, `system_admin` | none | aiops.automation.plans.create |
| GET | `/api/v1/aiops/automation/plans/:plan_id` | any | none | aiops.automation.plans.read |
| POST | `/api/v1/aiops/automation/plans/:plan_id/approve` | `operations_admin`, `system_admin` | none | aiops.automation.plans.approve |
| POST | `/api/v1/aiops/automation/plans/:plan_id/cancel` | `operations_admin`, `system_admin` | none | aiops.automation.plans.cancel |
| POST | `/api/v1/aiops/automation/plans/:plan_id/execute` | `operations_admin`, `system_admin` | none | aiops.automation.plans.execute |
| POST | `/api/v1/aiops/automation/plans/:plan_id/preview` | `operations_admin`, `system_admin` | none | aiops.automation.plans.preview |
| GET | `/api/v1/aiops/automation/plans/:plan_id/verification` | any | none | aiops.automation.verification.read |
| POST | `/api/v1/aiops/automation/plans/:plan_id/verify` | `operations_admin`, `system_admin` | none | aiops.automation.plans.verify |
| GET | `/api/v1/aiops/automation/runbooks` | any | none | aiops.automation.runbooks.list |
| GET | `/api/v1/aiops/correlation/cases` | any | none | aiops.correlation.cases.list |
| GET | `/api/v1/aiops/correlation/cases/:id` | any | none | aiops.correlation.cases.read |
| GET | `/api/v1/aiops/correlation/cases/:id/actions` | any | none | aiops.correlation.actions.list |
| GET | `/api/v1/aiops/correlation/cases/:id/graph` | any | none | aiops.correlation.graph.read |
| GET | `/api/v1/aiops/correlation/cases/timeline` | any | none | aiops.correlation.timeline.list |
| GET | `/api/v1/aiops/correlation/rules` | any | none | aiops.correlation.rules.list |
| GET | `/api/v1/aiops/insight` | any | none | aiops.insight.runbook.read |
| GET | `/api/v1/aiops/inspection/coverage` | any | none | aiops.inspection.coverage.read |
| GET | `/api/v1/aiops/inspection/plans` | any | none | aiops.inspection.plans.list |
| POST | `/api/v1/aiops/inspection/plans` | `operations_admin`, `system_admin` | none | aiops.inspection.plans.create |
| DELETE | `/api/v1/aiops/inspection/plans/:id` | `operations_admin`, `system_admin` | none | aiops.inspection.plans.delete |
| GET | `/api/v1/aiops/inspection/plans/:id` | any | none | aiops.inspection.plans.read |
| GET | `/api/v1/aiops/inspection/results` | any | none | aiops.inspection.results.list |
| GET | `/api/v1/aiops/inspection/results/:id` | any | none | aiops.inspection.results.read |
| GET | `/api/v1/aiops/inspection/rules/catalog` | any | none | aiops.inspection.rules_catalog.read |
| POST | `/api/v1/aiops/inspection/run` | `operations_admin`, `system_admin` | none | aiops.inspection.run_once |
| GET | `/api/v1/aiops/inspection/tasks` | any | none | aiops.inspection.tasks.list |
| GET | `/api/v1/aiops/inspection/tasks/:id` | any | none | aiops.inspection.tasks.read |
| GET | `/api/v1/aiops/investigator/cases/:case_id/investigations` | any | none | aiops.investigator.investigations.list |
| POST | `/api/v1/aiops/investigator/cases/:case_id/investigations` | any | none | aiops.investigator.investigations.generate |
| GET | `/api/v1/aiops/investigator/investigations/:id` | any | none | aiops.investigator.investigations.read |
| GET | `/api/v1/aiops/investigator/runbooks` | any | none | aiops.investigator.runbooks.list |
| GET | `/api/v1/aiops/overview` | any | none | aiops.overview.read |
| GET | `/api/v1/aiops/quality-report` | any | none | aiops.quality_report.read |
| POST | `/api/v1/aiops/quality-report/run` | `operations_admin`, `system_admin` | none | aiops.quality_report.run |
| GET | `/api/v1/aiops/signals` | any | none | aiops.signals.list |
| GET | `/api/v1/aiops/signals/catalog` | any | none | - |
| GET | `/api/v1/aiops/slos` | any | none | aiops.slo.definitions.list |
| POST | `/api/v1/aiops/slos` | `operations_admin`, `system_admin` | none | aiops.slo.definitions.create |
| DELETE | `/api/v1/aiops/slos/:id` | `operations_admin`, `system_admin` | none | aiops.slo.definitions.delete |
| GET | `/api/v1/aiops/slos/:id` | any | none | aiops.slo.definitions.read |
| PATCH | `/api/v1/aiops/slos/:id` | `operations_admin`, `system_admin` | none | aiops.slo.definitions.update |
| POST | `/api/v1/aiops/slos/:id/evaluate` | `operations_admin`, `system_admin` | none | aiops.slo.evaluate |
| GET | `/api/v1/aiops/slos/:id/evaluations` | any | none | aiops.slo.evaluations.list |
| GET | `/api/v1/aiops/slos/templates` | any | none | aiops.slo.templates.list |
| GET | `/api/v1/aiops/topology/changes` | any | none | aiops.topology.changes.list |
| GET | `/api/v1/aiops/topology/graph` | any | none | aiops.topology.graph.read |
| GET | `/api/v1/alert-routes` | any | none | - |
| POST | `/api/v1/alert-routes` | `operations_admin`, `system_admin` | none | alert_route.route.create |
| DELETE | `/api/v1/alert-routes/:id` | `operations_admin`, `system_admin` | none | alert_route.route.delete |
| PATCH | `/api/v1/alert-routes/:id` | `operations_admin`, `system_admin` | none | alert_route.route.update |
| GET | `/api/v1/alert-routes/deliveries` | `security_auditor`, `system_admin` | none | - |
| GET | `/api/v1/alert-routes/inhibits` | any | none | - |
| POST | `/api/v1/alert-routes/inhibits` | `operations_admin`, `system_admin` | none | alert_route.inhibit.create |
| DELETE | `/api/v1/alert-routes/inhibits/:id` | `operations_admin`, `system_admin` | none | alert_route.inhibit.delete |
| GET | `/api/v1/alert-routes/receivers` | any | none | - |
| POST | `/api/v1/alert-routes/receivers` | `operations_admin`, `system_admin` | none | alert_route.receiver.create |
| DELETE | `/api/v1/alert-routes/receivers/:id` | `operations_admin`, `system_admin` | none | alert_route.receiver.delete |
| GET | `/api/v1/alert-routes/silences` | any | none | - |
| POST | `/api/v1/alert-routes/silences` | `operations_admin`, `system_admin` | none | alert_route.silence.create |
| DELETE | `/api/v1/alert-routes/silences/:id` | `operations_admin`, `system_admin` | none | alert_route.silence.delete |
| GET | `/api/v1/app-catalog/plans` | any | none | app_catalog.plans.list |
| GET | `/api/v1/app-catalog/plans/:plan_id` | any | none | app_catalog.plans.read |
| POST | `/api/v1/app-catalog/plans/:plan_id/execute` | `operations_admin`, `system_admin` | none | app_catalog.plans.execute |
| POST | `/api/v1/app-catalog/plans/preview` | `operations_admin`, `system_admin` | none | app_catalog.plans.preview |
| GET | `/api/v1/app-catalog/repositories` | any | none | app_catalog.repositories.list |
| POST | `/api/v1/app-catalog/repositories` | `operations_admin`, `system_admin` | none | app_catalog.repositories.create |
| DELETE | `/api/v1/app-catalog/repositories/:repo_id` | `operations_admin`, `system_admin` | none | app_catalog.repositories.delete |
| GET | `/api/v1/app-catalog/repositories/:repo_id` | any | none | app_catalog.repositories.read |
| GET | `/api/v1/app-catalog/repositories/:repo_id/charts` | any | none | app_catalog.charts.list |
| GET | `/api/v1/app-catalog/repositories/:repo_id/charts/:chart_name` | any | none | app_catalog.charts.read |
| GET | `/api/v1/audit-logs` | `security_auditor`, `system_admin` | none | - |
| GET | `/api/v1/audit-logs/export` | `security_auditor`, `system_admin` | none | audit.export |
| POST | `/api/v1/auth/login` | any | none | auth.login |
| POST | `/api/v1/auth/logout` | any | none | auth.logout |
| GET | `/api/v1/auth/me` | any | none | - |
| GET | `/api/v1/auth/me/grants` | any | none | - |
| GET | `/api/v1/auth/oidc/callback` | any | none | auth.oidc.callback |
| GET | `/api/v1/auth/oidc/login` | any | none | auth.oidc.login |
| POST | `/api/v1/auth/oidc/logout` | any | none | auth.oidc.logout |
| POST | `/api/v1/auth/password-change` | any | none | auth.password.change |
| POST | `/api/v1/auth/refresh` | any | none | auth.refresh |
| GET | `/api/v1/auth/sessions` | any | none | - |
| DELETE | `/api/v1/auth/sessions/:session_id` | any | none | auth.session.revoke |
| POST | `/api/v1/auth/sessions/revoke-others` | any | none | auth.sessions.revoke_others |
| POST | `/api/v1/backup-plans/:plan_id/execute` | `operations_admin`, `system_admin` | none | backup.execute |
| POST | `/api/v1/capability/logs` | any | none | capability.logs.query |
| GET | `/api/v1/capability/metrics` | any | none | capability.metrics.query |
| GET | `/api/v1/capability/providers` | `operations_admin`, `system_admin` | none | capability.providers.list |
| GET | `/api/v1/capability/providers/:name` | `operations_admin`, `system_admin` | none | capability.providers.get |
| GET | `/api/v1/clusters` | any | none | - |
| POST | `/api/v1/clusters` | `system_admin` | none | cluster.create |
| DELETE | `/api/v1/clusters/:cluster_id` | `system_admin` | cluster | cluster.delete |
| GET | `/api/v1/clusters/:cluster_id` | any | cluster | - |
| PATCH | `/api/v1/clusters/:cluster_id` | `system_admin` | cluster | cluster.enabled.update |
| GET | `/api/v1/clusters/:cluster_id/alert-rules` | any | cluster | - |
| POST | `/api/v1/clusters/:cluster_id/alert-rules` | `operations_admin`, `system_admin` | cluster | alert_rule.create |
| DELETE | `/api/v1/clusters/:cluster_id/alert-rules/:rule_id` | `operations_admin`, `system_admin` | cluster | alert_rule.delete |
| GET | `/api/v1/clusters/:cluster_id/alert-rules/:rule_id` | any | cluster | - |
| PATCH | `/api/v1/clusters/:cluster_id/alert-rules/:rule_id` | `operations_admin`, `system_admin` | cluster | alert_rule.update |
| GET | `/api/v1/clusters/:cluster_id/alerts` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/alerts/:alert_id` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/api-resources` | any | cluster | kubernetes.api_resources.read |
| GET | `/api/v1/clusters/:cluster_id/backup-plans` | any | cluster | - |
| POST | `/api/v1/clusters/:cluster_id/backup-plans/preview` | `operations_admin`, `system_admin` | cluster | backup.preview |
| GET | `/api/v1/clusters/:cluster_id/backups` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/backups/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/clusterrolebindings` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/clusterrolebindings/:name` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/clusterroles` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/clusterroles/:name` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/configmaps` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/configmaps/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/copy-plans` | any | cluster | - |
| POST | `/api/v1/clusters/:cluster_id/copy-plans/preview` | `operations_admin`, `system_admin` | cluster | copyops.preview |
| PUT | `/api/v1/clusters/:cluster_id/credentials` | `system_admin` | cluster | cluster.credentials.rotate |
| GET | `/api/v1/clusters/:cluster_id/cronjobs` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/cronjobs/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/custom-resources/:group/:version/:resource` | any | cluster | kubernetes.custom_resources.list |
| GET | `/api/v1/clusters/:cluster_id/custom-resources/:group/:version/:resource/:name` | any | cluster | kubernetes.custom_resources.read |
| GET | `/api/v1/clusters/:cluster_id/daemonsets` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/daemonsets/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/deployments` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/deployments/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/deployments/:namespace/:name/rollout/history` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/deployments/:namespace/:name/rollout/status` | any | namespace | - |
| POST | `/api/v1/clusters/:cluster_id/diagnoses` | any | cluster | diagnosis.run |
| POST | `/api/v1/clusters/:cluster_id/diagnoses/node_metrics` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/endpointslices` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/events` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/events/cockpit` | any | cluster | kubernetes.events.cockpit.read |
| GET | `/api/v1/clusters/:cluster_id/events/stream` | any | cluster | kubernetes.events.stream |
| GET | `/api/v1/clusters/:cluster_id/gitops/applications` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/gitops/applications/:name` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/gitops/capability` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/horizontalpodautoscalers` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/horizontalpodautoscalers/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/ingresses` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/ingresses/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/inspection/rules` | any | cluster | aiops.inspection.rules_effective.list |
| GET | `/api/v1/clusters/:cluster_id/jobs` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/jobs/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/limitranges` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/limitranges/:namespace/:name` | any | namespace | - |
| POST | `/api/v1/clusters/:cluster_id/logs/query` | any | cluster | monitoring.logs.query |
| GET | `/api/v1/clusters/:cluster_id/maintenance-plans` | any | cluster | - |
| POST | `/api/v1/clusters/:cluster_id/maintenance-plans/preview` | `operations_admin`, `system_admin` | cluster | maintenance.preview |
| GET | `/api/v1/clusters/:cluster_id/metrics/history` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/metrics/history/evaluate` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/metrics/nodes` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/metrics/pods` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/monitoring/dashboard/:template` | any | cluster | monitoring.dashboard.read |
| GET | `/api/v1/clusters/:cluster_id/namespace-postures` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/namespace-postures/:namespace` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/namespaces` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/networkpolicies` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/networkpolicies/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/nodes` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/nodes/:name` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/operations` | any | cluster | - |
| POST | `/api/v1/clusters/:cluster_id/operations/preview` | `operations_admin`, `system_admin` | cluster | operation.preview |
| GET | `/api/v1/clusters/:cluster_id/persistentvolumeclaims` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/persistentvolumeclaims/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/persistentvolumes` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/persistentvolumes/:name` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/poddisruptionbudgets` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/poddisruptionbudgets/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/pods` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/pods/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/pods/:namespace/:name/all_logs` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/pods/:namespace/:name/containers` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/pods/:namespace/:name/logs` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/pods/:namespace/:name/logs_since` | any | namespace | - |
| POST | `/api/v1/clusters/:cluster_id/probe` | `operations_admin`, `system_admin` | cluster | cluster.probe |
| GET | `/api/v1/clusters/:cluster_id/replicasets` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/replicasets/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/resourcequotas` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/resourcequotas/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/resources/:kind/:namespace/:name/manifest` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/restore-plans` | any | cluster | - |
| POST | `/api/v1/clusters/:cluster_id/restore-plans/preview` | `operations_admin`, `system_admin` | cluster | restore.preview |
| GET | `/api/v1/clusters/:cluster_id/rolebindings` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/rolebindings/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/roles` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/roles/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/secrets` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/secrets/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/serviceaccounts` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/serviceaccounts/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/services` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/services/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/statefulsets` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/statefulsets/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/storageclasses` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/storageclasses/:name` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/velero/backups` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/velero/backups/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/clusters/:cluster_id/velero/capability` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/velero/restores` | any | cluster | - |
| GET | `/api/v1/clusters/:cluster_id/velero/restores/:namespace/:name` | any | namespace | - |
| GET | `/api/v1/copy-plans` | any | none | - |
| GET | `/api/v1/copy-plans/:plan_id` | `operations_admin`, `system_admin` | none | - |
| POST | `/api/v1/copy-plans/:plan_id/execute` | `operations_admin`, `system_admin` | none | copyops.execute |
| GET | `/api/v1/diagnoses` | any | none | - |
| GET | `/api/v1/diagnoses/:diagnosis_id` | any | none | - |
| PATCH | `/api/v1/diagnoses/:diagnosis_id` | `operations_admin`, `system_admin` | none | diagnosis.status.update |
| PATCH | `/api/v1/diagnoses/:diagnosis_id/assignment` | `operations_admin`, `system_admin` | none | diagnosis.assignment.update |
| GET | `/api/v1/diagnoses/:diagnosis_id/explanations` | any | none | - |
| POST | `/api/v1/diagnoses/:diagnosis_id/explanations` | `operations_admin`, `system_admin` | none | diagnosis.ai_explanation.create |
| POST | `/api/v1/diagnoses/:diagnosis_id/feedback` | `operations_admin`, `system_admin` | none | diagnosis.feedback.create |
| GET | `/api/v1/diagnoses/:diagnosis_id/remediations` | any | none | - |
| POST | `/api/v1/diagnoses/:diagnosis_id/remediations/preview` | `operations_admin`, `system_admin` | none | remediation.preview |
| GET | `/api/v1/diagnoses/:diagnosis_id/replay` | any | none | diagnosis.replay.read |
| GET | `/api/v1/diagnoses/summary` | any | none | - |
| DELETE | `/api/v1/federation/clusters/:cluster_id` | `operations_admin`, `system_admin` | cluster | federation.cluster.deregister |
| POST | `/api/v1/federation/clusters/:cluster_id/demote` | `operations_admin`, `system_admin` | cluster | federation.cluster.demote |
| GET | `/api/v1/federation/clusters/:cluster_id/events` | any | cluster | federation.cluster.events.list |
| POST | `/api/v1/federation/clusters/:cluster_id/heartbeat` | `operations_admin`, `system_admin` | cluster | federation.cluster.heartbeat |
| POST | `/api/v1/federation/clusters/:cluster_id/promote` | `operations_admin`, `system_admin` | cluster | federation.cluster.promote |
| PATCH | `/api/v1/federation/clusters/:cluster_id/status` | `operations_admin`, `system_admin` | cluster | federation.cluster.status.update |
| POST | `/api/v1/federation/clusters/register` | `operations_admin`, `system_admin` | none | federation.cluster.register |
| GET | `/api/v1/federation/events` | any | none | federation.events.list |
| GET | `/api/v1/federation/overview` | any | none | federation.overview.read |
| GET | `/api/v1/federation/resources/summary` | any | none | federation.resources.summary.read |
| GET | `/api/v1/fleet/health` | any | none | - |
| GET | `/api/v1/fleet/resources/search` | any | none | - |
| GET | `/api/v1/fleet/resources/search/filters` | any | none | - |
| POST | `/api/v1/fleet/resources/search/filters` | any | none | global_search_filter.create |
| DELETE | `/api/v1/fleet/resources/search/filters/:filter_id` | any | none | global_search_filter.delete |
| PATCH | `/api/v1/fleet/resources/search/filters/:filter_id` | any | none | global_search_filter.update |
| GET | `/api/v1/health/live` | any | none | - |
| GET | `/api/v1/health/ready` | any | none | - |
| GET | `/api/v1/incidents` | any | none | - |
| POST | `/api/v1/incidents` | `operations_admin`, `system_admin` | none | incident.create |
| GET | `/api/v1/incidents/:incident_id` | any | none | - |
| PATCH | `/api/v1/incidents/:incident_id` | `operations_admin`, `system_admin` | none | incident.status.update |
| PATCH | `/api/v1/incidents/:incident_id/assignment` | `operations_admin`, `system_admin` | none | incident.assignment.update |
| POST | `/api/v1/incidents/:incident_id/chat` | any | none | incident.chat.create |
| GET | `/api/v1/incidents/:incident_id/context` | any | none | incident.context.get |
| GET | `/api/v1/incidents/:incident_id/evidence` | any | none | incident.evidence.get |
| GET | `/api/v1/incidents/:incident_id/export` | any | none | incident.export |
| POST | `/api/v1/incidents/:incident_id/followers` | `operations_admin`, `system_admin` | none | incident.follower.add |
| DELETE | `/api/v1/incidents/:incident_id/followers/:user_id` | `operations_admin`, `system_admin` | none | incident.follower.remove |
| POST | `/api/v1/incidents/:incident_id/notes` | `operations_admin`, `system_admin` | none | incident.note.create |
| PUT | `/api/v1/incidents/:incident_id/postmortem` | `operations_admin`, `system_admin` | none | incident.postmortem.update |
| GET | `/api/v1/incidents/:incident_id/postmortem/export` | any | none | incident.postmortem.export |
| GET | `/api/v1/incidents/:incident_id/runbook` | any | none | incident.runbook.get |
| GET | `/api/v1/incidents/:incident_id/summary` | any | none | incident.summary.read |
| POST | `/api/v1/incidents/batch-assign` | `operations_admin`, `system_admin` | none | incident.assignment.batch |
| GET | `/api/v1/incidents/metrics` | any | none | - |
| GET | `/api/v1/incidents/summary` | any | none | - |
| GET | `/api/v1/incidents/templates` | any | none | - |
| POST | `/api/v1/maintenance-plans/:plan_id/execute` | `operations_admin`, `system_admin` | none | maintenance.execute |
| GET | `/api/v1/notification-deliveries` | `security_auditor`, `system_admin` | none | - |
| POST | `/api/v1/notification-deliveries/:delivery_id/retry` | `system_admin` | none | notification.delivery.retry |
| POST | `/api/v1/optimization/capacity/analyze` | any | none | optimization.capacity.analyze |
| POST | `/api/v1/optimization/capacity/preview` | any | none | optimization.capacity.preview |
| POST | `/api/v1/optimization/cis/analyze` | any | none | optimization.cis.analyze |
| POST | `/api/v1/optimization/deprecated-api/analyze` | any | none | optimization.deprecated_api.analyze |
| POST | `/api/v1/optimization/finops/analyze` | any | none | optimization.finops.analyze |
| POST | `/api/v1/optimization/gitops/analyze` | any | none | optimization.gitops.analyze |
| POST | `/api/v1/optimization/hpa/analyze` | any | none | optimization.hpa.analyze |
| POST | `/api/v1/optimization/image/analyze` | any | none | optimization.image.analyze |
| POST | `/api/v1/optimization/ingress/analyze` | any | none | optimization.ingress.analyze |
| POST | `/api/v1/optimization/network/analyze` | any | none | optimization.network.analyze |
| POST | `/api/v1/optimization/pdb/analyze` | any | none | optimization.pdb.analyze |
| POST | `/api/v1/optimization/policy/analyze` | any | none | optimization.policy.analyze |
| GET | `/api/v1/optimization/posture/cluster` | any | none | posture.cluster.report |
| GET | `/api/v1/promotions` | any | none | - |
| GET | `/api/v1/promotions/:promotion_id` | any | none | - |
| POST | `/api/v1/promotions/:promotion_id/execute` | `operations_admin`, `system_admin` | none | - |
| POST | `/api/v1/promotions/preview` | `operations_admin`, `system_admin` | none | - |
| POST | `/api/v1/remediations/:remediation_id/execute` | `operations_admin`, `system_admin` | none | remediation.execute |
| POST | `/api/v1/restore-plans/:plan_id/execute` | `operations_admin`, `system_admin` | none | restore.execute |
| GET | `/api/v1/users` | `system_admin` | none | - |
| POST | `/api/v1/users` | `system_admin` | none | user.create |
| PATCH | `/api/v1/users/:user_id` | `system_admin` | none | user.update |
| GET | `/api/v1/users/:user_id/cluster-grants` | `system_admin` | none | user.cluster_grant.list |
| POST | `/api/v1/users/:user_id/cluster-grants` | `system_admin` | none | user.cluster_grant.create |
| DELETE | `/api/v1/users/:user_id/cluster-grants/:cluster_id` | `system_admin` | cluster | user.cluster_grant.delete |
| GET | `/api/v1/users/:user_id/namespace-grants` | `system_admin` | none | user.namespace_grant.list |
| POST | `/api/v1/users/:user_id/namespace-grants` | `system_admin` | none | user.namespace_grant.create |
| DELETE | `/api/v1/users/:user_id/namespace-grants/:cluster_id/:namespace` | `system_admin` | namespace | user.namespace_grant.delete |
| POST | `/api/v1/users/:user_id/password-reset` | `system_admin` | none | user.password.reset |
| GET | `/api/v1/users/assignable` | `operations_admin`, `system_admin` | none | - |
| GET | `/api/v1/workspaces` | any | none | workspaces.list |
| POST | `/api/v1/workspaces` | `system_admin` | none | workspaces.create |
| DELETE | `/api/v1/workspaces/:workspace_id` | `system_admin` | workspace | workspaces.delete |
| GET | `/api/v1/workspaces/:workspace_id` | any | workspace | workspaces.read |
| PATCH | `/api/v1/workspaces/:workspace_id` | any | workspace | workspaces.update |
| DELETE | `/api/v1/workspaces/:workspace_id/memberships` | any | workspace | workspaces.memberships.remove |
| GET | `/api/v1/workspaces/:workspace_id/memberships` | any | workspace | workspaces.memberships.list |
| POST | `/api/v1/workspaces/:workspace_id/memberships` | any | workspace | workspaces.memberships.add |
| GET | `/api/v1/workspaces/:workspace_id/monitoring/dashboard` | any | workspace | monitoring.dashboard.read |
| GET | `/api/v1/workspaces/:workspace_id/quota` | any | workspace | workspaces.quota.read |
| PUT | `/api/v1/workspaces/:workspace_id/quota` | any | workspace | workspaces.quota.set |
| GET | `/api/v1/workspaces/:workspace_id/role-bindings` | any | workspace | workspaces.role_bindings.list |
| POST | `/api/v1/workspaces/:workspace_id/role-bindings` | any | workspace | workspaces.role_bindings.grant |
| DELETE | `/api/v1/workspaces/:workspace_id/role-bindings/:user_id` | any | workspace | workspaces.role_bindings.revoke |
| GET | `/api/v1/workspaces/:workspace_id/role-bindings/audit` | any | workspace | workspaces.role_bindings.audit.list |
