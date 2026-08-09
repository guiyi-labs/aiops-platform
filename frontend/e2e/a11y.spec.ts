import AxeBuilder from '@axe-core/playwright'
import { test, expect, type Page } from '@playwright/test'

import { mockAnonymousAuth, mockAuthenticatedAPI } from './api-fixtures'

const consoleErrors: string[] = []
const scannedRoutes = ['/', '/clusters', '/topology', '/posture', '/optimization', '/search', '/events']

async function expectNoSeriousViolations(page: Page) {
  await expect(page.locator('body')).toBeVisible()
  // 等路由过渡与卡片入场动画 settle，避免骨架/半透明态被扫成错误。
  await page.waitForTimeout(600)
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21aa'])
    .analyze()
  const violations = results.violations.filter((violation) => violation.impact === 'critical' || violation.impact === 'serious')
  expect(
    violations,
    JSON.stringify(violations.map((violation) => ({ id: violation.id, impact: violation.impact, nodes: violation.nodes.length, help: violation.help })), null, 2),
  ).toEqual([])
}

test.beforeEach(async ({ page }) => {
  consoleErrors.length = 0
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text())
  })
  await mockAuthenticatedAPI(page)
})

test.afterEach(() => {
  expect(consoleErrors).toEqual([])
})

for (const route of scannedRoutes) {
  test(`a11y scan: ${route}`, async ({ page }) => {
    await page.goto(route)
    await expectNoSeriousViolations(page)
  })
}

test('a11y scan: /login', async ({ page }) => {
  await mockAnonymousAuth(page)
  await page.goto('/login')
  await expectNoSeriousViolations(page)
})
