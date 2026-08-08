import AxeBuilder from '@axe-core/playwright'
import { test, expect } from '@playwright/test'

const consoleErrors: string[] = []
const scannedRoutes = ['/', '/clusters', '/topology', '/posture', '/optimization', '/search', '/events']

test.beforeEach(({ page }) => {
  consoleErrors.length = 0
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text())
  })
})

test.afterEach(() => {
  expect(consoleErrors).toEqual([])
})

for (const route of scannedRoutes) {
  test(`a11y scan: ${route}`, async ({ page }) => {
    await page.goto(route)
    await expect(page.locator('body')).toBeVisible()
    // 等路由过渡与卡片入场动画 settle，避免骨架/半透明态被扫成错误。
    await page.waitForTimeout(600)
    const results = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'wcag21aa'])
      .analyze()
    const violations = results.violations.filter((v) => v.impact === 'critical' || v.impact === 'serious')
    expect(
      violations,
      JSON.stringify(violations.map((v) => ({ id: v.id, impact: v.impact, nodes: v.nodes.length, help: v.help })), null, 2),
    ).toEqual([])
  })
}