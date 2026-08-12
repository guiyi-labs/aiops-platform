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
    if (method === 'POST' && path === '/api/v1/incidents') {
      await fulfillJSON(route, { ...incident, title: 'Pod pending', severity: 'warning' }, 201)
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
