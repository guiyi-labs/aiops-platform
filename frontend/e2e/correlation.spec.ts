import { test, expect, type Route } from '@playwright/test'

import { mockAuthenticatedAPI } from './api-fixtures'

const consoleErrors: string[] = []

function fulfillJSON(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

const caseItem = {
  id: 42,
  case_key: 'web-0-oom-restart',
  cluster_id: 1,
  rule_id: 'pod_oom_killed_restart',
  correlation_version: '1.0',
  primary_resource: { cluster_id: 1, namespace: 'default', kind: 'Pod', name: 'web-0' },
  status: 'active',
  confidence: 'confirmed',
  evidence_completeness: 0.92,
  factors: [
    { dimension: 'signal', weight: 0.6, evidence_refs: ['signal:1'], reason: 'OOMKilled 信号与重启事件同窗口' },
  ],
  first_observed_at: '2026-08-12T08:00:00Z',
  last_observed_at: '2026-08-12T08:05:00Z',
  diagnosis_ids: [],
  created_at: '2026-08-12T08:00:00Z',
  updated_at: '2026-08-12T08:05:00Z',
}

const caseView = {
  case: caseItem,
  signal_links: [
    {
      id: 1,
      case_id: 42,
      signal_occurrence_id: 10,
      relation: 'caused',
      signal_id: 'sig-oom-1',
      producer: 'kubelet',
      observed_at: '2026-08-12T08:00:30Z',
      coverage: 'complete',
      window_start: '2026-08-12T07:55:00Z',
      window_end: '2026-08-12T08:05:00Z',
      created_at: '2026-08-12T08:00:30Z',
    },
  ],
  resource_links: [],
  change_candidates: [],
  generated_at: '2026-08-12T08:06:00Z',
  incident: null,
}

const promotedIncident = {
  id: 9,
  number: 'INC-000009',
  title: '关联案例事故 web-0-oom-restart',
  source_type: 'correlation',
  source_ref: 'correlation:42',
  cluster_id: 1,
  severity: 'high',
  status: 'open',
  summary: '从关联案例 #42 提升的事故工作区',
  created_at: '2026-08-12T08:10:00Z',
  updated_at: '2026-08-12T08:10:00Z',
}

test.beforeEach(async ({ page }) => {
  consoleErrors.length = 0
  page.on('console', (msg) => {
    // Chromium 对任何非 2xx 响应都会自动记录 "Failed to load resource"；
    // 去重用例有意触发 409，属预期行为，过滤后其余 console error 仍严格断言。
    if (msg.type() === 'error' && !msg.text().includes('status of 409')) consoleErrors.push(msg.text())
  })
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/clusters', (route) => fulfillJSON(route, { items: [{ id: 1, name: 'demo-cluster', enabled: true }], total: 1, remaining: 0 }))
  await page.route('**/api/v1/aiops/correlation/cases**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const method = route.request().method()
    if (method === 'GET' && path === '/api/v1/aiops/correlation/cases') {
      await fulfillJSON(route, { items: [caseItem], total: 1, remaining: 0 })
      return
    }
    if (method === 'GET' && path === '/api/v1/aiops/correlation/cases/42') {
      await fulfillJSON(route, caseView)
      return
    }
    if (method === 'GET' && path === '/api/v1/aiops/correlation/cases/42/actions') {
      await fulfillJSON(route, { items: [], total: 0, remaining: 0 })
      return
    }
    await fulfillJSON(route, { items: [], total: 0, remaining: 0 })
  })
  await page.route('**/api/v1/incidents', async (route) => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON()
      if (body?.source_type === 'correlation' && body?.source_ref === 'correlation:42') {
        await fulfillJSON(route, promotedIncident, 201)
        return
      }
    }
    await fulfillJSON(route, { items: [], total: 0, remaining: 0 })
  })
})

test.afterEach(() => {
  expect(consoleErrors).toEqual([])
})

test('M108 deep link: ?case_id= focuses case detail and promotes to incident', async ({ page }) => {
  await page.goto('/aiops/correlation?case_id=42')
  await expect(page.locator('.case-detail-title .context-label')).toContainText('CASE #42')
  await expect(page.locator('.case-detail-title h2')).toContainText('Pod/web-0')
  await expect(page.locator('.case-detail-panel')).toContainText('sig-oom-1')
  await expect(page.locator('.case-detail-panel')).toContainText('kubelet')

  const promote = page.locator('.case-detail-actions .action-button')
  await expect(promote).toBeEnabled()
  await promote.click()
  await expect(page.locator('.notice-message')).toContainText('已创建事故工作区 INC-000009')
  await expect(page.locator('.linked-incident')).toContainText('已关联事故 INC-000009')

  // bidirectional deep link: incident workspace entry navigates to /incidents
  await page.locator('.linked-incident').click()
  await expect(page).toHaveURL(/\/incidents/)
})

test('M108 promote dedup: SOURCE_ALREADY_USED surfaces stable notice', async ({ page }) => {
  await page.route('**/api/v1/incidents', (route) => fulfillJSON(route, {
    code: 'SOURCE_ALREADY_USED',
    message: 'correlation:42 already linked to an incident workspace',
    request_id: 'req-dup-1',
  }, 409))

  await page.goto('/aiops/correlation?case_id=42')
  await expect(page.locator('.case-detail-title .context-label')).toContainText('CASE #42')
  await page.locator('.case-detail-actions .action-button').click()
  await expect(page.locator('.notice-message')).toContainText('该关联案例已存在关联的事故工作区')
  await expect(page.locator('.linked-incident')).toHaveCount(0)
})
