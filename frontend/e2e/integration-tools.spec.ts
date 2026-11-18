import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { mockAuthenticatedAPI } from './api-fixtures'

test('runtime preparation reports partial collection and exercises gateway validation without execution', async ({ page }) => {
  await mockAuthenticatedAPI(page)
  await page.route('**/api/v1/integrations/tools', route => route.fulfill({ json: { transport: 'stdio', read_only: true, items: [{ name: 'get_diagnosis', method: 'GET', path: '/api/v1/diagnoses/{diagnosis_id}', arguments: [{ name: 'diagnosis_id', required: true }] }] } }))
  await page.route('**/api/v1/integrations/tools/validate', route => {
    const body = route.request().postDataJSON()
    const allowed = Object.keys(body.arguments).length === 1 && body.arguments.diagnosis_id === '1'
    return route.fulfill({ json: { allowed, executed: false, ...(allowed ? { method: 'GET', path: '/api/v1/diagnoses/1' } : { reason: 'unknown argument or unacceptable value' }) } })
  })
  await page.route('**/api/v1/health/ready', route => route.fulfill({ json: { status: 'ready', service: 'aiops', version: 'dev' } }))
  await page.route('**/api/v1/clusters/1/metrics/nodes*', route => route.fulfill({ status: 503, json: { message: 'metrics unavailable' } }))
  await page.goto('/settings/tools')
  await expect(page.locator('.tool-catalog')).toContainText('get_diagnosis')
  await page.getByRole('button', { name: '校验参数', exact: true }).click()
  await expect(page.locator('.tool-result')).toContainText('参数校验通过')
  await expect(page.locator('.tool-result')).toContainText('未向后端发起工具请求')
  await page.getByRole('button', { name: '载入未声明参数' }).click()
  await page.getByRole('button', { name: '校验参数', exact: true }).click()
  await expect(page.locator('.tool-result')).toContainText('调用已被拦截')
  await page.getByRole('button', { name: '载入越界路径' }).click()
  await page.getByRole('button', { name: '校验参数', exact: true }).click()
  await expect(page.locator('.tool-result')).toContainText('调用已被拦截')
  await page.getByLabel('工具调用参数 JSON').fill('{invalid')
  await page.getByRole('button', { name: '校验参数', exact: true }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  await page.getByRole('button', { name: '检查演示环境' }).click()
  await expect(page.locator('.check-grid article')).toHaveCount(4)
  await expect(page.locator('.check-grid')).toContainText('尚未启用')
  await expect(page.getByRole('button', { name: '导出检查结果' })).toBeVisible()
  await page.getByRole('button', { name: '载入合法参数' }).click()
  for (const theme of ['theme-light', 'theme-dark']) {
    await page.evaluate(value => { document.documentElement.classList.remove('theme-light', 'theme-dark'); document.documentElement.classList.add(value) }, theme)
    await page.waitForTimeout(700)
    const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze()
    expect(result.violations.filter(item => ['serious', 'critical'].includes(item.impact ?? ''))).toEqual([])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  await expect(page.locator('.navigation .nav-row')).toHaveCount(5)
})
