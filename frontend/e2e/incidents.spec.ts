import { test, expect, type Route } from '@playwright/test'

import { mockAuthenticatedAPI } from './api-fixtures'

const consoleErrors: string[] = []

function fulfillJSON(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

const incident = {
  id: 7,
  number: 'INC-000007',
  title: 'web-0 CrashLoopBackOff',
  source_type: 'finding',
  source_ref: 'finding:1:pod.crash_loop_backoff.v1:Pod:default:web-0',
  cluster_id: 1,
  resource: { kind: 'Pod', namespace: 'default', name: 'web-0', uid: 'pod-7' },
  severity: 'high',
  status: 'open',
  summary: '容器持续崩溃重启',
  postmortem: '',
  assignee: undefined,
  followers: [],
  timeline: [
    { id: 1, event_type: 'system', actor: { id: 0, name: 'system' }, content: 'incident created from finding source finding:1:pod.crash_loop_backoff.v1:Pod:default:web-0', created_at: '2026-08-12T08:00:00Z' },
  ],
  version: 1,
  observed_at: '2026-08-12T08:00:00Z',
  sla_due_at: '2026-08-12T12:00:00Z',
  resolved_at: undefined,
  overdue: false,
  created_at: '2026-08-12T08:00:00Z',
  updated_at: '2026-08-12T08:00:00Z',
}

function resolvedIncident(): typeof incident {
  return {
    ...incident,
    status: 'resolved',
    version: 4,
    assignee: { id: 1, name: 'Playwright Admin' },
    followers: [{ user_id: 1, name: 'Playwright Admin', added_at: '2026-08-12T08:10:00Z' }],
    postmortem: 'root cause: image tag drift',
    resolved_at: '2026-08-12T08:30:00Z',
    timeline: [
      ...incident.timeline,
      { id: 2, event_type: 'system', actor: { id: 1, name: 'Playwright Admin' }, content: 'status changed from open to confirmed: reproduced', created_at: '2026-08-12T08:05:00Z' },
      { id: 3, event_type: 'note', actor: { id: 1, name: 'Playwright Admin' }, content: 'waiting on rollout', created_at: '2026-08-12T08:15:00Z' },
      { id: 4, event_type: 'system', actor: { id: 1, name: 'Playwright Admin' }, content: 'status changed from confirmed to resolved', created_at: '2026-08-12T08:30:00Z' },
    ],
  }
}

const summary = { total: 1, open: 1, confirmed: 0, resolved: 0, dismissed: 0, overdue: 0 }

// M112 context cockpit: IncidentsView.vue dereferences cockpit.resource_context.scope
// unconditionally once cockpit is set, so the mock must be a legal IncidentContextCockpit
// (never the api-fixtures fallback, which lacks resource_context).
const cockpitContext = {
  resource_context: {
    scope: { cluster_id: 1, namespace: 'default', kind: 'Pod', name: 'web-0', source_type: 'finding' },
    observed_at: '2026-08-12T08:00:00Z',
    source: 'finding',
    freshness: { age_seconds: 120, as_of: '2026-08-12T08:02:00Z' },
    empty_sample: { count: 0, bounded: true, semantic: 'fail_closed' },
  },
  incident: {
    id: 7,
    number: 'INC-000007',
    title: 'web-0 CrashLoopBackOff',
    severity: 'high',
    status: 'open',
    summary: '容器持续崩溃重启',
    source_type: 'finding',
    resource: { kind: 'Pod', namespace: 'default', name: 'web-0', uid: 'pod-7' },
    version: 1,
    created_at: '2026-08-12T08:00:00Z',
    updated_at: '2026-08-12T08:00:00Z',
  },
  sla: { due_at: '2026-08-12T12:00:00Z', overdue: false, remaining: '3h 58m', deadline_text: '3h 58m 后截止' },
  health: { status: 'open', overdue: false, evidence_available: true, runbook_available: false, note_count: 0, system_event_count: 1 },
  evidence_sources: [{ source_type: 'finding', count: 1, deep_link: '/diagnoses/1' }],
  recent_events: [
    { id: 1, event_type: 'system', actor: { id: 0, name: 'system' }, content: 'incident created from finding source finding:1:pod.crash_loop_backoff.v1:Pod:default:web-0', created_at: '2026-08-12T08:00:00Z' },
  ],
  recommended_actions: [{ action: 'restart rollout', target_kind: 'Deployment', dry_run_first: true, summary: '触发滚动重启（dry-run 预览）' }],
}

// M112-3 cited AI summary: IncidentsView.vue reads incidentSummary.next_steps /
// incidentSummary.citations unconditionally once incidentSummary is set.
const incidentSummary = {
  incident_id: 7,
  resource_context: cockpitContext.resource_context,
  mode: 'deterministic',
  root_cause_candidate: 'image tag drift (deterministic gate)',
  impact: 'web-0 CrashLoopBackOff',
  evidence_summary: '从 finding:1:pod.crash_loop_backoff.v1 提取',
  next_steps: [],
  citations: [],
  provider: 'deterministic',
  model: 'gate',
  input_tokens: 0,
  output_tokens: 0,
  fail_closed: true,
  stage_gate_passed: true,
  stage_gate_reason: 'deterministic fallback',
}

test.beforeEach(async ({ page }) => {
  consoleErrors.length = 0
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text())
  })
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/users/assignable', (route) => fulfillJSON(route, { items: [{ id: 1, username: 'admin', display_name: 'Playwright Admin', roles: ['system_admin'], status: 'active' }], total: 1, remaining: 0 }))
  await page.route('**/api/v1/incidents**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const method = route.request().method()
    if (method === 'GET' && path === '/api/v1/incidents') {
      await fulfillJSON(route, { items: [{ ...incident, timeline: undefined, followers: undefined }], total: 1, remaining: 0 })
      return
    }
    if (method === 'GET' && path === '/api/v1/incidents/summary') {
      await fulfillJSON(route, summary)
      return
    }
    if (method === 'GET' && path === '/api/v1/incidents/templates') {
      await fulfillJSON(route, {
        templates: [
          { id: 'generic', name: '通用事故', description: '通用响应模板', source_types: ['finding', 'diagnosis', 'alert', 'inspection', 'signal', 'correlation'], default_title: '待确认的运营事故', default_severity: 'warning', default_summary: '记录影响范围、当前症状与初步判断。', steps: ['确认影响范围', '指定负责人'] },
          { id: 'node-not-ready', name: 'Node NotReady', description: '节点不可用响应模板', source_types: ['finding'], default_title: 'Node NotReady 事故', default_severity: 'high', default_summary: '节点失联或 Ready 状态异常。', steps: ['确认节点状态', '检查节点事件'] },
        ],
        severity_matrix: [{ severity: 'critical', target_minutes: 60 }, { severity: 'high', target_minutes: 240 }, { severity: 'warning', target_minutes: 1440 }, { severity: 'info', target_minutes: 4320 }],
      })
      return
    }
    if (method === 'POST' && path === '/api/v1/incidents') {
      await fulfillJSON(route, { ...incident, title: 'Pod pending', severity: 'warning' }, 201)
      return
    }
    if (route.request().method() === 'GET' && path === '/api/v1/incidents/7/evidence') {
      await fulfillJSON(route, { items: [
        { source_type: 'finding', source_ref: 'finding:1:pod.crash_loop_backoff.v1:Pod:default:web-0', title: 'web-0 CrashLoopBackOff', summary: '容器持续崩溃重启', deep_link: '/diagnoses/1', severity: 'high', resource: { kind: 'Pod', namespace: 'default', name: 'web-0' }, observed_at: '2026-08-12T08:00:00Z' },
        { source_type: 'alert', source_ref: 'alert:9', title: 'Node CPU high', summary: 'demo-node cpu > 2B', deep_link: '/alerts/9', severity: 'warning', resource: { kind: 'Node', namespace: '', name: 'demo-node' }, observed_at: '2026-08-12T08:05:00Z' },
      ] })
      return
    }
    if (route.request().method() === 'GET' && path === '/api/v1/incidents/7') {
      await fulfillJSON(route, incident)
      return
    }
    if (route.request().method() === 'PATCH' && path === '/api/v1/incidents/7') {
      const body = route.request().postDataJSON()
      if (body?.status === 'resolved') await fulfillJSON(route, resolvedIncident())
      else if (body?.status === 'open') await fulfillJSON(route, { ...resolvedIncident(), status: 'open', version: 5, resolved_at: undefined, timeline: [...resolvedIncident().timeline, { id: 5, event_type: 'system', actor: { id: 1, name: 'Playwright Admin' }, content: 'status changed from resolved to open: regression observed', created_at: '2026-08-12T08:40:00Z' }] })
      else await fulfillJSON(route, { ...incident, status: body?.status ?? 'confirmed', version: 2, timeline: [...incident.timeline, { id: 2, event_type: 'system', actor: { id: 1, name: 'Playwright Admin' }, content: `status changed from open to ${body?.status ?? 'confirmed'}`, created_at: '2026-08-12T08:05:00Z' }] })
      return
    }
    if (route.request().method() === 'PATCH' && path === '/api/v1/incidents/7/assignment') {
      await fulfillJSON(route, { ...incident, status: 'confirmed', version: 3, assignee: { id: 1, name: 'Playwright Admin' }, timeline: [...incident.timeline, { id: 2, event_type: 'system', actor: { id: 1, name: 'Playwright Admin' }, content: 'handoff from unassigned to Playwright Admin', created_at: '2026-08-12T08:05:00Z' }] })
      return
    }
    if (route.request().method() === 'POST' && path === '/api/v1/incidents/7/notes') {
      await fulfillJSON(route, { ...incident, status: 'confirmed', version: 4, assignee: { id: 1, name: 'Playwright Admin' }, timeline: [...incident.timeline, { id: 2, event_type: 'system', actor: { id: 1, name: 'Playwright Admin' }, content: 'status changed from open to confirmed', created_at: '2026-08-12T08:05:00Z' }, { id: 3, event_type: 'note', actor: { id: 1, name: 'Playwright Admin' }, content: 'waiting on rollout', created_at: '2026-08-12T08:15:00Z' }] })
      return
    }
    if (route.request().method() === 'POST' && path === '/api/v1/incidents/7/followers') {
      await fulfillJSON(route, { ...incident, status: 'confirmed', version: 4, assignee: { id: 1, name: 'Playwright Admin' }, followers: [{ user_id: 1, name: 'Playwright Admin', added_at: '2026-08-12T08:10:00Z' }], timeline: [...incident.timeline, { id: 4, event_type: 'system', actor: { id: 1, name: 'Playwright Admin' }, content: 'Playwright Admin is now following this incident', created_at: '2026-08-12T08:10:00Z' }] })
      return
    }
    if (route.request().method() === 'DELETE' && path === '/api/v1/incidents/7/followers/1') {
      await fulfillJSON(route, { ...incident, status: 'confirmed', version: 5, assignee: { id: 1, name: 'Playwright Admin' }, followers: [], timeline: [...incident.timeline, { id: 5, event_type: 'system', actor: { id: 1, name: 'Playwright Admin' }, content: 'Playwright Admin stopped following this incident', created_at: '2026-08-12T08:11:00Z' }] })
      return
    }
    if (route.request().method() === 'PUT' && path === '/api/v1/incidents/7/postmortem') {
      await fulfillJSON(route, resolvedIncident())
      return
    }
    if (route.request().method() === 'GET' && path === '/api/v1/incidents/7/postmortem/export') {
      await route.fulfill({ status: 200, contentType: 'text/markdown', headers: { 'Content-Disposition': 'attachment; filename="incident-INC-000007-postmortem.md"' }, body: '# INC-000007\n\n## Outcome\n' })
      return
    }
    if (route.request().method() === 'GET' && path === '/api/v1/incidents/7/context') {
      await fulfillJSON(route, cockpitContext)
      return
    }
    if (route.request().method() === 'GET' && path === '/api/v1/incidents/7/summary') {
      await fulfillJSON(route, incidentSummary)
      return
    }
    await fulfillJSON(route, { items: [], total: 0, remaining: 0 })
  })
})

test.afterEach(() => {
  expect(consoleErrors).toEqual([])
})

test('Incident list renders summary board and opens detail drawer', async ({ page }) => {
  await page.goto('/incidents')
  await expect(page.locator('.incident-stats')).toContainText('待确认')
  await expect(page.locator('.incident-stats .stat-card').first()).toContainText('1')

  const row = page.locator('.incident-title')
  await expect(row).toContainText('web-0 CrashLoopBackOff')
  await row.click()

  const drawer = page.locator('.incident-drawer')
  await expect(drawer).toBeVisible()
  await expect(drawer).toContainText('INC-000007')
  await expect(drawer.locator('.incident-timeline')).toContainText('incident created from finding')
})

test('Incident workflow: confirm, handoff, note, resolve, postmortem and reopen', async ({ page }) => {
  await page.goto('/incidents')
  await page.locator('.incident-title').click()

  const drawer = page.locator('.incident-drawer')

  // confirm open -> confirmed
  await drawer.locator('select[aria-label="目标状态"]').selectOption('confirmed')
  await drawer.getByRole('button', { name: '提交' }).click()
  await expect(drawer.locator('.workflow-status')).toContainText('已确认')

  // handoff
  await drawer.locator('select[aria-label="选择负责人"]').selectOption('1')
  await drawer.getByRole('button', { name: '移交' }).click()
  await expect(drawer).toContainText('Playwright Admin')

  // note
  await drawer.locator('textarea[placeholder="记录新的备注…"]').fill('waiting on rollout')
  await drawer.getByRole('button', { name: '添加备注' }).click()
  await expect(drawer.locator('.incident-timeline')).toContainText('waiting on rollout')

  // resolve + postmortem
  await drawer.locator('select[aria-label="目标状态"]').selectOption('resolved')
  await drawer.getByRole('button', { name: '提交' }).click()
  await expect(drawer.locator('.workflow-status')).toContainText('已解决')
  await drawer.locator('textarea[placeholder="记录根因、处理过程与后续改进…"]').fill('root cause: image tag drift')
  await drawer.getByRole('button', { name: '保存复盘' }).click()
  await expect(drawer).toContainText('root cause: image tag drift')

  // narrative: postmortem metrics render for resolved incidents
  await expect(drawer).toContainText('复盘视图')
  await expect(drawer.locator('.postmortem-metrics')).toContainText('SLA 达标')
  await expect(drawer.locator('.postmortem-metrics')).toContainText('解决耗时')
  await expect(drawer.locator('.postmortem-metrics')).toContainText('人工备注')
  const downloadPromise = page.waitForEvent('download')
  await drawer.getByRole('button', { name: '导出 Markdown' }).click()
  await expect((await downloadPromise).suggestedFilename()).toContain('postmortem')

  // narrative: timeline filter by type (resolved timeline has 3 system + 1 note)
  const timelineFilter = drawer.locator('.filter-tabs').nth(1)
  await expect(timelineFilter).toContainText('全部 (4)')
  await timelineFilter.getByRole('button', { name: '备注 (1)' }).click()
  await expect(drawer.locator('.incident-timeline li')).toHaveCount(1)
  await expect(drawer.locator('.incident-timeline')).toContainText('waiting on rollout')
  await timelineFilter.getByRole('button', { name: '系统 (3)' }).click()
  await expect(drawer.locator('.incident-timeline li')).toHaveCount(3)
  await expect(drawer.locator('.incident-timeline')).toContainText('status changed from confirmed to resolved')
  await timelineFilter.getByRole('button', { name: '全部 (4)' }).click()

  // narrative: evidence filter by source (finding + alert)
  const evidenceFilter = drawer.locator('.filter-tabs').first()
  await expect(evidenceFilter).toContainText('全部 (2)')
  await evidenceFilter.getByRole('button', { name: /告警实例/ }).click()
  await expect(drawer.locator('.evidence-card')).toHaveCount(1)
  await expect(drawer.locator('.evidence-card')).toContainText('Node CPU high')
  await evidenceFilter.getByRole('button', { name: '全部 (2)' }).click()
  await expect(drawer.locator('.evidence-card')).toHaveCount(2)

  // reopen
  await drawer.locator('select[aria-label="目标状态"]').selectOption('open')
  await drawer.getByRole('button', { name: '提交' }).click()
  await expect(drawer.locator('.workflow-status')).toContainText('待确认')
})

test('Incident create dialog validates source identity', async ({ page }) => {
  await page.goto('/incidents')
  await page.getByRole('button', { name: /新建事故/ }).click()
  const form = page.locator('.incident-form')
  await expect(form).toBeVisible()
  await form.locator('select[aria-label="响应模板"]').selectOption('node-not-ready')
  await expect(form.getByLabel('标题')).toHaveValue('Node NotReady 事故')
  await expect(form.locator('select[aria-label="严重级别"]')).toHaveValue('high')
  await expect(form.getByLabel('摘要')).toHaveValue('节点失联或 Ready 状态异常。')
  const createButton = form.getByRole('button', { name: '创建' })
  await expect(createButton).toBeDisabled()
  await form.locator('input[placeholder^="finding:"]').fill('finding:1:pod.pending.v1:Pod:default:web-2')
  await form.locator('input[type="number"]').fill('1')
  await expect(createButton).toBeEnabled()
  await createButton.click()
  await expect(page.locator('.incident-form')).not.toBeVisible()
})

test('Viewer role cannot manage incidents', async ({ page }) => {
  await page.route('**/api/v1/auth/refresh', (route) => fulfillJSON(route, {
    access_token: 'viewer-token', token_type: 'Bearer', expires_in: 900,
    user: { id: 3, username: 'viewer', display_name: 'Viewer', roles: ['viewer'] },
  }))
  await page.goto('/incidents')
  await expect(page.getByRole('button', { name: /新建事故/ })).toHaveCount(0)
  await page.locator('.incident-title').click()
  const drawer = page.locator('.incident-drawer')
  await expect(drawer.getByRole('button', { name: '提交' })).toHaveCount(0)
  await expect(drawer.getByRole('button', { name: '移交' })).toHaveCount(0)
  await expect(drawer.getByRole('button', { name: '添加备注' })).toHaveCount(0)
})
