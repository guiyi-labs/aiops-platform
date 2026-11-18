import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { mockAuthenticatedAPI } from './api-fixtures'

function pod(name: string, phase = 'Running') {
  return { metadata: { name, namespace: 'default', uid: name }, spec: { containers: [{ name: 'app', image: 'example/app:v1' }] }, status: { phase, conditions: [], containerStatuses: [{ name: 'app', ready: phase === 'Running', restartCount: 0, state: {} }] } }
}
const pods = [pod('web-a'), pod('web-b', 'Pending')]

test.beforeEach(async ({ page }) => {
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/clusters/1/pods?*', route => route.fulfill({ json: { items: pods, remaining: 0 } }))
  await page.route('**/api/v1/clusters/1/pods/default/*', route => route.fulfill({ json: pods.find(item => route.request().url().endsWith(item.metadata.name)) }))
  await page.route('**/api/v1/clusters/1/metrics/history?*', route => route.fulfill({ status: 503, json: { message: 'metrics unavailable' } }))
})

test('list remains usable beside details, selection and browser history stay in sync', async ({ page }, testInfo) => {
  await page.goto('/workloads')
  const rows = page.locator('.resource-row')
  await rows.filter({ hasText: 'web-a' }).click()
  const detail = page.getByRole('region', { name: 'Pod 详情' })
  await expect(detail).toContainText('web-a')
  await expect(detail.locator('.resource-detail-stats')).toBeVisible()
  await expect(page.locator('.log-overlay')).toHaveCount(0)
  await expect(rows.filter({ hasText: 'web-b' }).locator('.resource-status')).toBeVisible()
  if (testInfo.project.name === 'Desktop Chrome') {
    const left = await page.locator('.resource-workbench').boundingBox()
    const right = await detail.boundingBox()
    expect(left!.x + left!.width).toBeLessThanOrEqual(right!.x)
    const status = await rows.filter({ hasText: 'web-b' }).locator('.resource-status').boundingBox()
    expect(status!.x + status!.width).toBeLessThan(right!.x)
  }
  await rows.filter({ hasText: 'web-b' }).press('Enter')
  await expect(detail.locator('h2')).toHaveText('web-b')
  await expect(rows.filter({ hasText: 'web-b' })).toHaveAttribute('aria-current', 'true')
  await expect(page.getByRole('link', { name: '完整详情' })).toHaveAttribute('href', '/clusters/1/resources/Pod/default/web-b')
  await page.goBack()
  await expect(detail.locator('h2')).toHaveText('web-a')
  await page.getByLabel('切换详情资源').selectOption('default/web-b')
  await expect(detail.locator('h2')).toHaveText('web-b')
  for (const theme of ['theme-light', 'theme-dark']) {
    await page.evaluate(value => { document.documentElement.classList.remove('theme-light', 'theme-dark'); document.documentElement.classList.add(value) }, theme)
    await page.waitForTimeout(700)
    await detail.scrollIntoViewIfNeeded()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const audit = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
    expect(audit.violations.filter(item => ['serious', 'critical'].includes(item.impact ?? '')).map(item => ({ id: item.id, nodes: item.nodes.map(node => ({ target: node.target, summary: node.failureSummary })) }))).toEqual([])
    await page.screenshot({ path: `../.artifacts/resource-browser-20261118/${testInfo.project.name}-${theme}.png`, fullPage: true })
  }
  await detail.getByRole('button', { name: '关闭详情' }).click()
  await expect(detail).toHaveCount(0)
  await expect(rows).toHaveCount(2)
})

test('diagnosis feedback stays with details through loading, no match, failure and retry', async ({ page }) => {
  let release!: () => void
  let attempt = 0
  const pending = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/clusters/1/diagnoses', async route => {
    attempt++
    if (attempt === 1) { await pending; await route.fulfill({ status: 422, json: { code: 'NO_RULE_MATCH', message: 'no rule match' } }) }
    else if (attempt === 2) await route.fulfill({ status: 503, json: { message: 'diagnosis offline' } })
    else await route.fulfill({ json: { id: 42, rule_id: 'pod-pending', summary: '调度证据需要检查', root_causes: [], recommendations: [], evidence: [] } })
  })
  await page.goto('/workloads?cluster=1&kind=Pod&namespace=default&name=web-b')
  const detail = page.getByRole('region', { name: 'Pod 详情' })
  const run = detail.getByRole('button', { name: '运行诊断', exact: true })
  await run.click()
  await expect(detail.getByRole('status')).toContainText('正在采集证据')
  await expect(run).toBeDisabled()
  release()
  await expect(detail.getByRole('status')).toContainText('未生成诊断记录')
  await expect(detail.getByRole('status')).toContainText('不代表资源一定健康')
  await run.click()
  await expect(detail.getByRole('alert')).toContainText('诊断请求失败')
  await run.click()
  await expect(detail.getByRole('status')).toContainText('调度证据需要检查')
  await detail.getByRole('button', { name: '查看诊断结论与证据' }).click()
  await expect(page.getByRole('dialog', { name: '规则诊断' })).toContainText('调度证据需要检查')
  await page.getByRole('button', { name: '关闭诊断' }).click()
  await expect(detail).toBeVisible()
})

test('switching resources ignores the previous diagnosis response', async ({ page }) => {
  let release!: () => void
  const pending = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/clusters/1/diagnoses', async route => {
    await pending
    await route.fulfill({ json: { id: 99, summary: 'OLD RESOURCE RESULT', root_causes: [], recommendations: [], evidence: [] } })
  })
  await page.goto('/workloads?cluster=1&kind=Pod&namespace=default&name=web-a')
  const detail = page.getByRole('region', { name: 'Pod 详情' })
  await detail.getByRole('button', { name: '运行诊断', exact: true }).click()
  await page.getByLabel('切换详情资源').selectOption('default/web-b')
  await expect(detail.locator('h2')).toHaveText('web-b')
  const response = page.waitForResponse('**/api/v1/clusters/1/diagnoses')
  release()
  await response
  await expect(detail.getByRole('button', { name: '运行诊断', exact: true })).toBeEnabled()
  await expect(page.getByText('OLD RESOURCE RESULT')).toHaveCount(0)
  await expect(detail.locator('.browser-diagnosis-feedback')).toHaveCount(0)
})

test('long lists scroll to another resource while its details stay open', async ({ page }) => {
  const many = Array.from({ length: 100 }, (_, index) => pod(`worker-${String(index).padStart(3, '0')}`))
  await page.route('**/api/v1/clusters/1/pods?*', route => route.fulfill({ json: { items: many, remaining: 0 } }))
  await page.route('**/api/v1/clusters/1/pods/default/*', route => route.fulfill({ json: many.find(item => route.request().url().endsWith(item.metadata.name)) }))
  await page.goto('/workloads?cluster=1&kind=Pod&namespace=default&name=worker-000')
  const list = page.getByTestId('pod-virtual-list')
  await expect(page.getByRole('region', { name: 'Pod 详情' })).toContainText('worker-000')
  await list.evaluate(element => { element.scrollTop = element.scrollHeight; element.dispatchEvent(new Event('scroll')) })
  const last = page.locator('.resource-row').filter({ hasText: 'worker-099' })
  await expect(last).toBeVisible()
  await last.click()
  await expect(page.getByRole('region', { name: 'Pod 详情' }).locator('h2')).toHaveText('worker-099')
  expect(await page.locator('.resource-row').count()).toBeLessThan(30)
})

test('cluster scoped nodes retain status and switch without a namespace', async ({ page }) => {
  const nodes = ['node-a', 'node-b'].map((name, index) => ({ metadata: { name }, spec: {}, status: { conditions: [{ type: 'Ready', status: index ? 'False' : 'True' }], addresses: [], nodeInfo: {}, allocatable: {}, capacity: {} } }))
  await page.route('**/api/v1/clusters/1/nodes?*', route => route.fulfill({ json: { items: nodes } }))
  await page.route('**/api/v1/clusters/1/nodes/*', route => route.fulfill({ json: nodes.find(item => route.request().url().endsWith(item.metadata.name)) }))
  await page.goto('/workloads?cluster=1&kind=Node&name=node-a')
  const detail = page.getByRole('region', { name: 'Node 详情' })
  await expect(detail.locator('h2')).toHaveText('node-a')
  const row = page.locator('.resource-row').filter({ hasText: 'node-b' })
  await expect(row.locator('.resource-status')).toHaveText('False')
  await expect(row.locator('.resource-status')).toBeVisible()
  await row.click()
  await expect(detail.locator('h2')).toHaveText('node-b')
  await expect(page.getByRole('link', { name: '完整详情' })).toHaveAttribute('href', '/clusters/1/resources/Node/_/node-b')
})
