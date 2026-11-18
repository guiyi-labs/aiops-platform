import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { mockAuthenticatedAPI } from './api-fixtures'

const list = (items: unknown[], total = items.length) => ({ items, total, remaining: total - items.length })

test.beforeEach(async ({ page }) => {
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/clusters', route => route.fulfill({ json: list([
    { id: 1, name: 'cluster-one', enabled: true }, { id: 2, name: 'cluster-two', enabled: true },
  ]) }))
  await page.route('**/api/v1/clusters/*/nodes?*', route => {
    const name = route.request().url().includes('/clusters/2/') ? 'worker-two' : 'worker-one'
    return route.fulfill({ json: list([{ metadata: { name }, spec: {}, status: { conditions: [{ type: 'Ready', status: 'True' }], allocatable: { cpu: '2', memory: '4Gi' } } }]) })
  })
  await page.route('**/api/v1/clusters/*/metrics/nodes?*', route => route.fulfill({ json: list([{ metadata: { name: 'worker-one' }, usage: { cpu: '500m', memory: '1Gi' } }]) }))
  await page.route('**/api/v1/clusters/*/metrics/history?*', route => {
    const url = new URL(route.request().url())
    const from = url.searchParams.get('from')!
    const to = url.searchParams.get('to')!
    return route.fulfill({ json: { from, to, series: { unit: url.searchParams.get('metric') === 'cpu' ? 'nanocores' : 'bytes' }, points: [0, 1, 2].map((index) => ({ value: 100_000_000 + index * 10_000_000, collected_at: new Date(Date.parse(to) - (2 - index) * 60_000).toISOString() })), coverage: { collections: 3, points: 3, missing: 0, failed: 0, partial: 0, unavailable: 0, timed_out: 0 }, truncated: false } })
  })
})

test('dashboard reads real metrics, changes node scope and preserves its topology link', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.resource-gauge').first()).toContainText('25%')
  await expect(page.getByRole('img', { name: /worker-one CPU趋势/ })).toBeVisible()
  await page.getByRole('button', { name: '内存', exact: true }).first().click()
  await expect(page.getByRole('img', { name: /worker-one 内存趋势/ })).toBeVisible()
  await page.getByLabel('当前诊断集群').selectOption('2')
  await expect(page.getByLabel('趋势节点')).toHaveValue('worker-two')
  await expect(page.getByRole('img', { name: /worker-two 内存趋势/ })).toBeVisible()
  await expect(page.getByRole('link', { name: '查看完整资源拓扑' })).toHaveAttribute('href', '/diagnoses?cluster=2&view=topology')
  await expect(page.locator('.cluster-map')).not.toContainText('worker-one')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('dashboard marks unavailable data and bounded samples', async ({ page }) => {
  await page.route('**/api/v1/clusters/*/metrics/nodes?*', route => route.fulfill({ status: 503, json: { error: { message: 'unavailable' } } }))
  await page.route('**/api/v1/clusters/*/events?*', route => route.fulfill({ json: list([{ type: 'Warning', metadata: { name: 'warning-one' }, reason: 'BackOff', message: 'container restarting', involvedObject: { kind: 'Pod', name: 'example' } }], 130) }))
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('部分数据暂不可用')
  await expect(page.locator('.resource-gauge').first()).toContainText('暂无指标')
  await expect(page.locator('.resource-gauge').first()).not.toContainText('0%')
  await expect(page.locator('.dashboard-footer')).toContainText('事件列表存在分页')
  await expect(page.locator('.warning-list')).toContainText('BackOff')
})

test('presentation supports keyboard exit, focus return, and accessible themes', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.resource-gauge').first()).toContainText('25%')
  for (const theme of ['theme-light', 'theme-dark']) {
    await page.evaluate(value => { document.documentElement.classList.remove('theme-light', 'theme-dark'); document.documentElement.classList.add(value) }, theme)
    await page.getByRole('button', { name: '全屏展示', exact: true }).click()
    await expect(page.getByRole('dialog', { name: '集群运行态势' })).toBeVisible()
    await page.keyboard.press('Tab')
    expect(await page.evaluate(() => !!document.activeElement?.closest('.dashboard'))).toBe(true)
    await page.waitForTimeout(700)
    const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
    expect(result.violations.filter(item => ['serious', 'critical'].includes(item.impact ?? '')), JSON.stringify(result.violations)).toEqual([])
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(page.getByRole('button', { name: '全屏展示', exact: true })).toBeFocused()
  }
})
