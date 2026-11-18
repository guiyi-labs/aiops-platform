import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { readFile } from 'node:fs/promises'
import { mockAuthenticatedAPI } from './api-fixtures'

const node = { metadata: { name: 'worker-1', uid: 'node-1', labels: { role: 'worker' } }, status: { conditions: [{ type: 'Ready', status: 'False' }], addresses: [], nodeInfo: {} } }

test('Node detail diagnoses the node name and keeps resource available when events fail', async ({ page }) => {
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/clusters/1/nodes/worker-1', route => route.fulfill({ json: node }))
  await page.route('**/api/v1/clusters/1/events?*', route => route.fulfill({ status: 503, json: { message: 'events offline' } }))
  let submitted: unknown
  await page.route('**/api/v1/clusters/1/diagnoses', route => { submitted = route.request().postDataJSON(); return route.fulfill({ json: { id: 42, summary: '节点尚未就绪' } }) })
  await page.goto('/clusters/1/resources/Node/_/worker-1')
  await expect(page.locator('.resource-identity')).toContainText('worker-1')
  await page.getByRole('button', { name: '相关事件', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('资源信息仍可查看')
  await page.getByRole('button', { name: '运行规则诊断', exact: true }).click()
  await expect(page.locator('.diagnosis-outcome')).toContainText('节点尚未就绪')
  expect(submitted).toEqual({ resource_kind: 'Node', namespace: '', name: 'worker-1' })
  await expect(page.getByRole('link', { name: '查看诊断结论与证据 →' })).toHaveAttribute('href', '/diagnoses?id=42')
  for (const theme of ['theme-light', 'theme-dark']) {
    await page.evaluate(value => { document.documentElement.classList.remove('theme-light', 'theme-dark'); document.documentElement.classList.add(value) }, theme)
    await page.waitForTimeout(700)
    const audit = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
    expect(audit.violations.filter(item => ['serious', 'critical'].includes(item.impact ?? ''))).toEqual([])
  }
})

test('Pod detail separates event kinds, filters multiline logs and downloads redacted manifest', async ({ page }) => {
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/clusters/1/pods/default/web', route => route.fulfill({ json: { metadata: { name: 'web', namespace: 'default', uid: 'pod-1' }, spec: {}, status: {} } }))
  await page.route('**/api/v1/clusters/1/events?*', route => route.fulfill({ json: { items: [
    { metadata: { uid: 'event-1' }, involvedObject: { kind: 'Pod', name: 'web', namespace: 'default' }, reason: 'BackOff', message: 'target pod' },
    { metadata: { uid: 'event-2' }, involvedObject: { kind: 'Service', name: 'web', namespace: 'default' }, reason: 'DifferentKind', message: 'wrong resource' },
  ], remaining: 0 } }))
  await page.route('**/api/v1/clusters/1/pods/default/web/containers', route => route.fulfill({ json: { items: [{ name: 'app', restart_count: 2 }] } }))
  await page.route('**/api/v1/clusters/1/pods/default/web/all_logs?*', route => route.fulfill({ json: { containers: [{ container: 'app', lines: [{ timestamp: '2026-11-18T00:00:00Z', message: 'first line' }, { timestamp: '2026-11-18T00:00:01Z', message: 'second error' }], truncated: false }] } }))
  await page.route('**/api/v1/clusters/1/resources/Pod/default/web/manifest', route => route.fulfill({ json: { kind: 'Pod', metadata: { name: 'web' }, spec: { secret: '<redacted>' } } }))
  await page.goto('/clusters/1/resources/Pod/default/web')
  await page.getByRole('button', { name: '相关事件', exact: true }).click()
  await expect(page.locator('.resource-event')).toHaveCount(1)
  await expect(page.locator('.resource-event')).toContainText('target pod')
  await page.getByRole('button', { name: '容器日志', exact: true }).click()
  await expect(page.getByLabel('容器日志内容')).toContainText('first line')
  await expect(page.getByLabel('容器日志内容')).toContainText('second error')
  await page.getByLabel('筛选日志内容').fill('error')
  await expect(page.getByLabel('容器日志内容')).not.toContainText('first line')
  const logsDownload = page.waitForEvent('download')
  await page.getByRole('button', { name: '下载容器日志' }).click()
  expect(await readFile((await (await logsDownload).path())!, 'utf8')).toContain('first line\n2026-11-18T00:00:01Z second error')
  await page.getByRole('button', { name: '脱敏清单', exact: true }).click()
  await expect(page.getByLabel('脱敏资源清单 JSON')).toContainText('<redacted>')
  const manifestDownload = page.waitForEvent('download')
  await page.getByRole('button', { name: '下载清单' }).click()
  expect(JSON.parse(await readFile((await (await manifestDownload).path())!, 'utf8')).spec.secret).toBe('<redacted>')
})

test('HorizontalPodAutoscaler is supported and no-rule-match is presented without a fake diagnosis', async ({ page }) => {
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/clusters/1/horizontalpodautoscalers/default/web', route => route.fulfill({ json: { metadata: { name: 'web', namespace: 'default' }, spec: {}, status: {} } }))
  let submitted: unknown
  await page.route('**/api/v1/clusters/1/diagnoses', route => { submitted = route.request().postDataJSON(); return route.fulfill({ status: 422, json: { code: 'NO_RULE_MATCH', message: 'no rule match' } }) })
  await page.goto('/clusters/1/resources/HorizontalPodAutoscaler/default/web')
  await page.getByRole('button', { name: '运行规则诊断' }).click()
  await expect(page.getByRole('status')).toContainText('未生成诊断记录')
  expect(submitted).toEqual({ resource_kind: 'HorizontalPodAutoscaler', namespace: 'default', name: 'web' })
  await expect(page.locator('.diagnosis-outcome')).toHaveCount(0)
})
