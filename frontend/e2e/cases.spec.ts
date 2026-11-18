import { test, expect } from '@playwright/test'
import { mockAuthenticatedAPI } from './api-fixtures'

test('Cases search opens the source diagnosis and offers no repair controls', async ({ page }) => {
  await mockAuthenticatedAPI(page)
  const queries: string[] = []
  await page.route('**/api/v1/aiops/knowledge**', route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/stats')) return route.fulfill({ json: { total: 1 } })
    queries.push(url.searchParams.get('q') ?? '')
    return route.fulfill({ json: { total: 1, items: [{
      id: 1, source_diagnosis_id: 42, rule_id: 'pod.image_pull.v1', severity: 'high',
      resource_kind: 'Pod', resource_name: 'web', summary: '镜像拉取失败',
      root_causes: ['镜像标签不存在'], recommendations: ['人工检查镜像标签'], noted_at: '2026-11-18T02:00:00Z',
    }] } })
  })
  await page.goto('/cases')
  await expect(page.locator('.case-card')).toContainText('镜像标签不存在')
  await page.getByRole('textbox', { name: '故障描述' }).fill('镜像拉取失败')
  await page.getByRole('button', { name: '检索案例', exact: true }).click()
  await expect.poll(() => queries.at(-1)).toBe('镜像拉取失败')
  await expect(page.getByRole('link', { name: /查看来源诊断与证据/ })).toHaveAttribute('href', '/diagnoses?id=42')
  await expect(page.locator('.navigation .nav-row')).toHaveCount(5)
  await expect(page.getByRole('button', { name: /修复|发布|执行/ })).toHaveCount(0)
})
