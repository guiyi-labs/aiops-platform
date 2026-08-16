import { test, expect, type Route } from '@playwright/test'

import { mockAuthenticatedAPI } from './api-fixtures'

function fulfillJSON(route: Route, body: unknown) {
  return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) })
}

const cluster = { id: 1, name: 'evidence-demo', created_at: '', updated_at: '', enabled: true }
const observedAt = '2026-08-10T10:00:00Z'

const optimizationFinding = {
  code: 'NO_MATCHING_PDB',
  severity: 'warning',
  summary: 'Deployment 没有匹配的 PDB 保护。',
  resource: { kind: 'Deployment', namespace: 'payments', name: 'api', uid: 'deployment-1' },
  details: { remediation: '创建匹配的 PodDisruptionBudget。' },
  observed_at: observedAt,
}

const postureReport = {
  cluster_id: 1,
  evaluated_at: observedAt,
  total_checks: 12,
  failed_checks: 1,
  passed_checks: 11,
  by_severity: { critical: 0, warning: 1, info: 0 },
  domains: [{ domain: 'pdb', total: 1, passed: 0, failed: 1, by_severity: { warning: 1 } }],
  findings: [{ ...optimizationFinding, domain: 'pdb' }],
}

const finopsRecommendation = {
  cluster_id: 1,
  namespace: 'payments',
  workload_kind: 'Deployment',
  workload_name: 'api',
  container_name: 'api',
  suggested_requests: { cpu_request: 100000000, cpu_limit: 500000000, mem_request: 134217728, mem_limit: 536870912 },
  suggested_limits: { cpu_request: 200000000, cpu_limit: 500000000, mem_request: 268435456, mem_limit: 536870912 },
  severity: 'warning',
  rationale: 'P95 使用量低于当前 request，建议下调资源申请。',
  code: 'FINOPS_OVERPROVISIONED',
  monthly_waste_usd: 12.5,
  replicas: 2,
}

const cisStatus = {
  cluster_id: 1,
  evaluated_at: observedAt,
  total: 3,
  failed: 1,
  passed: 2,
  by_severity: { critical: 0, warning: 1, info: 0 },
  by_family: { workload: 1 },
  findings: [optimizationFinding],
}

const inspectionTask = {
  id: 7,
  plan_id: 3,
  plan_name_snapshot: 'node-readiness',
  triggered_by: 'playwright',
  trigger_reason: 'manual',
  cluster_ids: [1],
  rule_codes: ['node_not_ready'],
  status: 'completed',
  started_at: observedAt,
  finished_at: observedAt,
  total_clusters: 1,
  completed_clusters: 1,
  finding_count: 1,
  error_summary: '',
  created_at: observedAt,
}

const inspectionResult = {
  id: 11,
  task_id: 7,
  cluster_id: 1,
  rule_code: 'node_not_ready',
  signal_code: 'NODE_READY_FALSE',
  severity: 'critical',
  state: 'active',
  namespace: '',
  resource_kind: 'Node',
  resource_name: 'worker-a',
  resource_uid: 'node-a',
  fingerprint: 'inspection-fingerprint-11',
  evidence: { condition: 'Ready=False', event: 'KubeletNotReady' },
  observed_at: observedAt,
}

// M113 inspection coverage: InspectionView renders Object.keys(coverage.by_severity)
// and coverage.trend unconditionally once coverage is set, so the mock must be a
// legal InspectionCoverageSummary (never the api-fixtures fallback).
const inspectionCoverage = {
  scope: 'cluster:1',
  observed_at: observedAt,
  window_days: 30,
  plan_total: 1,
  plan_enabled: 1,
  task_total: 1,
  task_completed: 1,
  task_failed: 0,
  task_scheduled: 0,
  task_manual: 1,
  finding_total: 1,
  distinct_rule_codes: 1,
  by_severity: { critical: 1 },
  rule_coverage: 1,
  trend: [{ day: observedAt.slice(0, 10), tasks: 1, findings: 1 }],
  fail_closed: false,
}

test.beforeEach(async ({ page }) => {
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/clusters', (route) => fulfillJSON(route, { items: [cluster], total: 1, remaining: 0 }))
  await page.route('**/api/v1/optimization/posture/cluster**', (route) => fulfillJSON(route, postureReport))
  await page.route('**/api/v1/optimization/finops/analyze', (route) => fulfillJSON(route, { cluster_id: 1, containers_evaluated: 1, containers_over_provisioned: 1, monthly_waste_usd: 12.5, cpu_idle_cores: 0.4, mem_idle_gb: 0.25, recommendations: [finopsRecommendation], evaluated_at: observedAt }))
  await page.route('**/api/v1/optimization/cis/analyze', (route) => fulfillJSON(route, cisStatus))
  await page.route('**/api/v1/aiops/inspection/rules/catalog', (route) => fulfillJSON(route, { items: [] }))
  await page.route('**/api/v1/aiops/inspection/plans', (route) => fulfillJSON(route, { items: [] }))
  await page.route('**/api/v1/aiops/inspection/tasks**', (route) => fulfillJSON(route, { items: [inspectionTask], total: 1 }))
  await page.route('**/api/v1/aiops/inspection/results**', (route) => fulfillJSON(route, { items: [inspectionResult], total: 1 }))
  await page.route('**/api/v1/aiops/inspection/coverage**', (route) => fulfillJSON(route, inspectionCoverage))
})

test('Posture evidence panel expands with source and recommendation', async ({ page }) => {
  await page.goto('/posture')
  const panel = page.locator('.posture-finding .finding-evidence-panel').first()
  await expect(panel).toBeVisible()
  await expect(panel.locator('.finding-evidence-count')).toContainText('1 条')
  await panel.getByRole('button', { name: /证据链/ }).click()
  await expect(panel.locator('.finding-evidence-body')).toBeVisible()
  await expect(panel).toContainText('创建匹配的 PodDisruptionBudget')
  await expect(panel).toContainText('Deployment/payments/api')
})

test('Optimization evidence panel is available in analyzer tabs', async ({ page }) => {
  await page.goto('/optimization')
  const finopsPanel = page.locator('.optimization-tab .finding-evidence-panel').first()
  await expect(finopsPanel).toBeVisible()
  await finopsPanel.getByRole('button', { name: /证据链/ }).click()
  await expect(finopsPanel.locator('.finding-evidence-body')).toBeVisible()
  await expect(finopsPanel).toContainText('只读建议')

  await page.getByRole('button', { name: 'CIS 合规' }).click()
  const cisPanel = page.locator('.optimization-tab .finding-evidence-panel').first()
  await expect(cisPanel).toBeVisible()
  await cisPanel.getByRole('button', { name: /证据链/ }).click()
  await expect(cisPanel.locator('.finding-evidence-body')).toContainText('创建匹配的 PodDisruptionBudget')
})

test('Inspection evidence panel stays within the viewport', async ({ page }) => {
  await page.goto('/inspection')
  await page.locator('.task-row').first().click()
  const panel = page.locator('.result-table .finding-evidence-panel').first()
  await expect(panel).toBeVisible()
  await panel.getByRole('button', { name: /证据链/ }).click()
  await expect(panel.locator('.finding-evidence-body')).toBeVisible()
  await expect(panel).toContainText('Ready=False')

  const box = await panel.boundingBox()
  const viewport = page.viewportSize()
  expect(box).not.toBeNull()
  expect(viewport).not.toBeNull()
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width + 1)
})
