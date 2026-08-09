import { test, expect } from '@playwright/test'

import { mockAnonymousAuth, mockAuthenticatedAPI } from './api-fixtures'

const consoleErrors: string[] = []

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

test('App shell renders the operations console', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('aside, nav[aria-label*="主导航"], nav[aria-label*="导航"]').first()).toBeVisible()
  await expect(page).toHaveURL(/\/(login|search|topology|events|diagnoses|$)/)
})

test('Login page presents auth form when unauthenticated', async ({ page }) => {
  await mockAnonymousAuth(page)
  await page.goto('/login')
  await expect(page.locator('button[type="submit"], button:has-text("登录")').first()).toBeVisible()
  const capabilities = page.getByRole('list', { name: '平台核心能力' })
  await expect(capabilities).toContainText('多集群治理')
  await expect(capabilities).toContainText('证据优先')
  await expect(capabilities).toContainText('审计闭环')
  await expect(capabilities).not.toContainText(/12|186|99|实时概况/)
})

test('Dashboard renders operational cards', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('h2, h1').first()).toBeVisible()
  await expect(page.locator('h2, h1').first()).toContainText(/集群|态势|总览|健康|AIOps/)
  await expect(page.locator('select').first()).toBeVisible()
})

test('Sidebar exposes navigation items', async ({ page }) => {
  await page.goto('/')
  const nav = page.locator('nav[aria-label*="导航"], nav')
  await expect(nav).toBeVisible()
  const item = nav.locator('button[class*="nav-item"]').first()
  await expect(item).toBeVisible()
})

test('Optimization page renders a heading', async ({ page }) => {
  await page.goto('/optimization')
  const heading = page.locator('h2, h1').first()
  await expect(heading).toBeVisible()
  await expect(heading).toContainText(/优化|FinOps|CIS|API/)
})

test('Posture page renders aggregate posture', async ({ page }) => {
  await page.goto('/posture')
  const heading = page.locator('h2, h1').first()
  await expect(heading).toBeVisible()
  await expect(heading).toContainText(/治理|态势|Posture|集群/)
})

test('Topology page renders without crashing', async ({ page }) => {
  await page.goto('/topology')
  const heading = page.locator('h2, h1').first()
  await expect(heading).toBeVisible()
  await expect(heading).toContainText(/拓扑|Topology|总览/)
})
