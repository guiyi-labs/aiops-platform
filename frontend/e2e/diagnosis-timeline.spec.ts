import { test, expect, type Route } from '@playwright/test'

import { mockAuthenticatedAPI } from './api-fixtures'

const consoleErrors: string[] = []

function fulfillJSON(route: Route, body: unknown) {
  return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) })
}

const replayView = {
  schema: 'aiops.diagnosis-replay/v1',
  diagnosis_id: 42,
  rule_id: 'node.not_ready.v1',
  severity: 'critical',
  resource: { kind: 'Node', namespace: '', name: 'worker-1', uid: 'node-1' },
  observed_at: '2026-07-26T10:02:00Z',
  steps: [
    { index: 0, stage: 'evidence', category: 'resource_state', type: 'node_condition', summary: 'Ready = False', ref: 'diagnosis:42:evidence:0', occurred_at: '2026-07-26T10:00:00Z', missing: false },
    { index: 1, stage: 'evidence', category: 'resource_state', type: 'node_condition', summary: 'MemoryPressure = True', ref: 'diagnosis:42:evidence:1', occurred_at: '2026-07-26T10:01:00Z', missing: false },
    { index: 2, stage: 'diagnosis_created', type: 'diagnosis_created', summary: '诊断创建 · node.not_ready.v1（critical）', ref: 'diagnosis:42', occurred_at: '2026-07-26T10:02:00Z', missing: false, detail: { rule_id: 'node.not_ready.v1', severity: 'critical', status: 'confirmed' } },
    { index: 3, stage: 'activity', type: 'status_transition', summary: 'open → confirmed', ref: 'activity:10', occurred_at: '2026-07-26T10:05:00Z', missing: false, detail: { actor: 'operator-a', comment: '确认根因' } },
    { index: 4, stage: 'remediation', type: 'remediation_created', summary: '受控动作预览 · deployment.rollout_restart（succeeded）', ref: 'remediation:plan-1', occurred_at: '2026-07-26T10:06:00Z', missing: false, detail: { action: 'deployment.rollout_restart', status: 'succeeded', target_name: 'worker-app' } },
    { index: 5, stage: 'remediation', type: 'remediation_executed', summary: '受控动作执行 · deployment.rollout_restart → succeeded', ref: 'remediation:plan-1:executed', occurred_at: '2026-07-26T10:07:00Z', missing: false, detail: { action: 'deployment.rollout_restart', status: 'succeeded', target_name: 'worker-app' } },
  ],
  stages: [
    { stage: 'diagnosis_created', label: '诊断创建', count: 1 },
    { stage: 'evidence', label: '证据采集', count: 2 },
    { stage: 'activity', label: '状态与协作', count: 1 },
    { stage: 'remediation', label: '受控动作', count: 2 },
  ],
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
  actions: [
    { kind: 'advisory', title: '检查 Ready Condition 的 Reason、Message 与最近转换时间' },
  ],
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
    if (route.request().method() === 'GET' && path === '/api/v1/diagnoses/42/replay') {
      await fulfillJSON(route, replayView)
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

  const evidencePanel = drawer.locator('.finding-evidence-panel')
  await expect(evidencePanel).toBeVisible()
  await expect(evidencePanel.locator('.finding-evidence-count')).toContainText('2 条')
  await evidencePanel.getByRole('button', { name: /证据链/ }).click()
  await expect(evidencePanel.locator('.finding-evidence-body')).toBeVisible()
  await expect(evidencePanel.locator('.finding-evidence-list')).toContainText('Ready = False')
  await expect(evidencePanel.locator('.finding-evidence-list')).toContainText('MemoryPressure = True')

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

  const actionArea = drawer.locator('.action-area')
  await expect(actionArea).toBeVisible()
  await expect(actionArea.locator('.action-item')).toHaveCount(1)
  await expect(actionArea.locator('.action-kind')).toHaveText('只读建议')
  await expect(actionArea).toContainText('检查 Ready Condition')
})


test('Diagnosis replay panel walks the stored insight chain', async ({ page }) => {
  await page.goto('/diagnoses')
  await page.locator('.diagnosis-history-row').first().click()

  const drawer = page.locator('.diagnosis-drawer')
  await expect(drawer).toBeVisible()
  const panel = drawer.locator('.replay-panel')
  await expect(panel).toBeVisible()
  await expect(panel.locator('h3')).toContainText('回放模式 · 6 步')
  await expect(panel.locator('.replay-schema')).toHaveText('aiops.diagnosis-replay/v1')

  // 初始空态：等待用户开始回放
  await expect(panel.locator('.compact-empty')).toContainText('按 ▶ 播放')

  // 进度条 seek 到第 2 步（确定性，不依赖定时器）
  await panel.locator('.replay-scrubber').evaluate((el) => {
    const input = el as HTMLInputElement
    input.value = '2'
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  const step = panel.locator('.replay-step')
  await expect(step).toBeVisible()
  await expect(step).toContainText('MemoryPressure = True')

  // 上一步回到第 1 步
  await panel.getByRole('button', { name: /上一步/ }).click()
  await expect(step).toContainText('Ready = False')

  // 按阶段筛选：证据采集（2 步）
  const chip = panel.locator('.replay-chip').filter({ hasText: '证据采集' })
  await chip.click()
  await expect(panel.locator('.replay-progress')).toHaveText('1 / 2')
  await expect(panel.locator('.replay-step')).toContainText('Ready = False')
  await expect(panel.locator('.replay-chip.active')).toContainText('证据采集')

  // 取消筛选恢复全链路
  await chip.click()
  await expect(panel.locator('.replay-progress')).toHaveText('0 / 6')
  await expect(panel.locator('.replay-chip')).toHaveCount(4)

  // 播放/暂停
  await panel.getByRole('button', { name: '播放' }).click()
  await expect(panel.getByRole('button', { name: '暂停' })).toBeVisible()
  await panel.getByRole('button', { name: '暂停' }).click()
  await expect(panel.getByRole('button', { name: '播放' })).toBeVisible()

  // 受控动作阶段可回溯（remediation created/executed 均存在）
  await panel.locator('.replay-chip').filter({ hasText: '受控动作' }).click()
  await expect(panel.locator('.replay-progress')).toHaveText('1 / 2')
  await expect(panel.locator('.replay-step')).toContainText('受控动作预览')
  await panel.locator('.replay-step').getByText('deployment.rollout_restart').first().click()
  await expect(panel.locator('.replay-detail')).toContainText('rollout_restart')
})

const brokenReplayDetail = {
  ...detail,
  id: 44,
  summary: 'Node 未处于 Ready 状态（replay 不可用场景）。',
}

test('Diagnosis replay degrades gracefully when the replay API is unavailable', async ({ page }) => {
  await page.route('**/api/v1/diagnoses**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (route.request().method() === 'GET' && path === '/api/v1/diagnoses') {
      await fulfillJSON(route, { items: [{ ...brokenReplayDetail, timeline: undefined, root_cause_card: undefined }], total: 1, remaining: 0 })
      return
    }
    if (route.request().method() === 'GET' && path === '/api/v1/diagnoses/44') {
      await fulfillJSON(route, brokenReplayDetail)
      return
    }
    if (route.request().method() === 'GET' && path === '/api/v1/diagnoses/44/replay') {
      await route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ code: 'DIAGNOSIS_NOT_FOUND', message: 'not found' }) })
      return
    }
    await fulfillJSON(route, { items: [], total: 0, remaining: 0 })
  })

  await page.goto('/diagnoses')
  await page.locator('.diagnosis-history-row').first().click()

  const drawer = page.locator('.diagnosis-drawer')
  await expect(drawer).toBeVisible()
  await expect(drawer.locator('.replay-error')).toContainText('回放模式不可用')
  // 其余详情仍正常渲染，不因回放失败而空白
  await expect(drawer.locator('.root-cause-card')).toContainText('Node 未处于 Ready 状态')

  // 404 触发的浏览器资源加载日志是该异常场景的预期副作用；移除后保持
  // console-error 零容忍门禁（应用本身未产生任何 console.error）。
  const resourceError = 'Failed to load resource: the server responded with a status of 404 (Not Found)'
  const errorIndex = consoleErrors.indexOf(resourceError)
  if (errorIndex >= 0) consoleErrors.splice(errorIndex, 1)
})

const podDetail = {
  id: 43,
  cluster_id: 1,
  rule_id: 'pod.oom_killed.v1',
  severity: 'critical',
  resource: { kind: 'Pod', namespace: 'demo', name: 'memory-api', uid: 'pod-oom' },
  status: 'open',
  summary: 'Pod 容器曾因 OOMKilled 被系统终止，存在内存压力或内存限制配置问题。',
  root_causes: ['容器实际内存使用超过 limits.memory'],
  recommendations: ['对比容器内存使用趋势与 requests/limits 配置'],
  evidence: [],
  actions: [
    { kind: 'advisory', title: '对比容器内存使用趋势与 requests/limits 配置' },
    { kind: 'controlled_action', title: 'Rollout restart 目标 Deployment', action: 'deployment.rollout_restart', requires_dry_run: true, requires_confirmation: true },
  ],
  timeline: [],
  observed_at: '2026-07-17T02:10:00Z',
  sla_due_at: '2026-07-17T02:40:00Z',
  overdue: false,
  created_at: '2026-07-17T02:10:00Z',
  updated_at: '2026-07-17T02:10:00Z',
}

test('Diagnosis with controlled action surfaces the degraded dependency note', async ({ page }) => {
  await page.route('**/api/v1/diagnoses**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (route.request().method() === 'GET' && path === '/api/v1/diagnoses') {
      await fulfillJSON(route, { items: [{ ...podDetail, timeline: undefined, actions: undefined }], total: 1, remaining: 0 })
      return
    }
    if (route.request().method() === 'GET' && path === '/api/v1/diagnoses/43') {
      await fulfillJSON(route, podDetail)
      return
    }
    await fulfillJSON(route, { items: [], total: 0, remaining: 0 })
  })
  await page.goto('/diagnoses')
  await page.locator('.diagnosis-history-row').first().click()

  const drawer = page.locator('.diagnosis-drawer')
  await expect(drawer).toBeVisible()
  const actionArea = drawer.locator('.action-area')
  await expect(actionArea.locator('.action-item')).toHaveCount(2)
  await expect(actionArea.locator('.action-kind.controlled_action')).toHaveText('受控动作')
  await expect(actionArea.locator('.action-item.controlled_action')).toContainText('Rollout restart 目标 Deployment')
  // open status → dependency note (未 confirmed)
  const note = actionArea.locator('.action-area-note')
  await expect(note).toBeVisible()
  await expect(note).toContainText('confirmed 状态')
  await expect(drawer.locator('.remediation-preview-form')).toHaveCount(0)
})


test('Diagnosis drawer deep links navigate to resource and audit targets', async ({ page }) => {
  await page.goto('/diagnoses')
  await page.locator('.diagnosis-history-row').first().click()
  const drawer = page.locator('.diagnosis-drawer')
  await expect(drawer).toBeVisible()

  const deepLinks = drawer.locator('.deep-links')
  await expect(deepLinks).toBeVisible()
  // Node diagnosis → cluster-scoped resource detail path
  const resourceButton = deepLinks.getByRole('button', { name: '资源详情' })
  await expect(resourceButton).toBeVisible()
  await resourceButton.click()
  await expect(page).toHaveURL('/clusters/1/resources/Node/_/worker-1')
})

test('Diagnosis drawer offers workloads and audit deep links', async ({ page }) => {
  await page.goto('/diagnoses')
  await page.locator('.diagnosis-history-row').first().click()
  const drawer = page.locator('.diagnosis-drawer')
  await expect(drawer).toBeVisible()

  const deepLinks = drawer.locator('.deep-links')
  const workloadsButton = deepLinks.getByRole('button', { name: '工作负载与相关事件' })
  await expect(workloadsButton).toBeVisible()
  await workloadsButton.click()
  await expect(page).toHaveURL(/\/workloads\?cluster=1&kind=Node&name=worker-1/)
})
