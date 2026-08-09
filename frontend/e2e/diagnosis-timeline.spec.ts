import { test, expect, type Route } from '@playwright/test'

import { mockAuthenticatedAPI } from './api-fixtures'

const consoleErrors: string[] = []

function fulfillJSON(route: Route, body: unknown) {
  return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) })
}

const detail = {
  id: 42,
  cluster_id: 1,
  rule_id: 'node.not_ready.v1',
  severity: 'critical',
  resource: { kind: 'Node', namespace: '', name: 'worker-1', uid: 'node-1' },
  status: 'open',
  summary: 'Node 未处于 Ready 状态，可能无法继续接收或承载工作负载。',
  root_causes: ['kubelet 或容器运行时异常'],
  recommendations: ['检查 Ready Condition 的 Reason、Message 与最近转换时间'],
  evidence: [
    { type: 'node_condition', source: 'node.status.conditions', content: { type: 'Ready', status: 'False', reason: 'KubeletNotReady', message: 'runtime is unavailable', last_transition_time: '2026-07-26T10:00:00Z' } },
    { type: 'node_condition', source: 'node.status.conditions', content: { type: 'MemoryPressure', status: 'True', reason: 'KubeletHasInsufficientMemory', message: 'memory pressure', last_transition_time: '2026-07-26T10:01:00Z' } },
  ],
  timeline: [
    { index: 0, category: 'resource_state', type: 'node_condition', source: 'node.status.conditions', ref: 'diagnosis:42:evidence:0', integrity: 'a'.repeat(64), occurred_at: '2026-07-26T10:00:00Z', missing: false, summary: 'Ready = False' },
    { index: 1, category: 'resource_state', type: 'node_condition', source: 'node.status.conditions', ref: 'diagnosis:42:evidence:1', integrity: 'b'.repeat(64), occurred_at: '2026-07-26T10:01:00Z', missing: false, summary: 'MemoryPressure = True' },
  ],
  root_cause_card: {
    conclusion: 'Node 未处于 Ready 状态，可能无法继续接收或承载工作负载。',
    severity: 'critical',
    status: 'open',
    first_observed_at: '2026-07-26T10:00:00Z',
    confidence: 'deterministic',
    confidence_source: 'node.not_ready.v1',
    resource: { kind: 'Node', name: 'worker-1' },
    key_evidence_refs: ['diagnosis:42:evidence:0', 'diagnosis:42:evidence:1'],
  },
  observed_at: '2026-07-26T10:02:00Z',
  sla_due_at: '2026-07-26T10:32:00Z',
  overdue: false,
  created_at: '2026-07-26T10:02:00Z',
  updated_at: '2026-07-26T10:02:00Z',
}

test.beforeEach(async ({ page }) => {
  consoleErrors.length = 0
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text())
  })
  await mockAuthenticatedAPI(page)

  await page.route('**/api/v1/clusters', (route) => fulfillJSON(route, { items: [{ id: 1, name: 'demo', created_at: '', updated_at: '' }], total: 1, remaining: 0 }))
  await page.route('**/api/v1/diagnoses**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (route.request().method() === 'GET' && path === '/api/v1/diagnoses') {
      await fulfillJSON(route, { items: [{ ...detail, timeline: undefined, root_cause_card: undefined }], total: 1, remaining: 0 })
      return
    }
    if (route.request().method() === 'GET' && path === '/api/v1/diagnoses/42') {
      await fulfillJSON(route, detail)
      return
    }
    await fulfillJSON(route, { items: [], total: 0, remaining: 0 })
  })
})

test.afterEach(() => {
  expect(consoleErrors).toEqual([])
})

test('Diagnosis detail shows root cause card and evidence timeline', async ({ page }) => {
  await page.goto('/diagnoses')
  const row = page.locator('.diagnosis-history-row').first()
  await expect(row).toBeVisible()
  await row.click()

  const drawer = page.locator('.diagnosis-drawer')
  await expect(drawer).toBeVisible()

  const card = drawer.locator('.root-cause-card')
  await expect(card).toBeVisible()
  await expect(card).toContainText('Node 未处于 Ready 状态')
  await expect(card.locator('.root-cause-confidence')).toHaveText('deterministic')
  await expect(card.locator('.root-cause-meta')).toContainText('node.not_ready.v1')
  await expect(card.locator('.root-cause-meta')).toContainText('diagnosis:42:evidence:0')

  const timeline = drawer.locator('.evidence-timeline')
  await expect(timeline).toBeVisible()
  await expect(timeline.locator(':scope > article')).toHaveCount(2)
  await expect(timeline.locator(':scope > article').first()).toContainText('Ready = False')
  await expect(timeline.locator(':scope > article').nth(1)).toContainText('MemoryPressure = True')
  await expect(timeline.locator('.raw-evidence summary')).toContainText('原始证据 JSON')
  const entryTimes = await timeline.locator(':scope > article time').allTextContents()
  expect(entryTimes).toHaveLength(2)
  expect(entryTimes[0] < entryTimes[1]).toBe(true)
})