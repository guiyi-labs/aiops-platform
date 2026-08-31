import { test, expect } from '@playwright/test'

import { mockAnonymousAuth, mockAuthenticatedAPI, mockSuccessfulLogin } from './api-fixtures'

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
  await expect(page.locator('.app-shell')).toHaveCount(1)
  await expect(page.locator('aside.sidebar')).toHaveCount(1)
  await expect(page.locator('header.topbar')).toHaveCount(1)
  await expect(page.locator('aside, nav[aria-label*="主导航"], nav[aria-label*="导航"]').first()).toBeVisible()
  await expect(page).toHaveURL(/\/(login|search|topology|events|diagnoses|$)/)
})

test('Login page presents auth form when unauthenticated', async ({ page }) => {
  await mockAnonymousAuth(page)
  await page.goto('/login')
  await expect(page.locator('.app-shell')).toHaveCount(0)
  await expect(page.locator('button[type="submit"], button:has-text("登录")').first()).toBeVisible()
  const capabilities = page.locator('.login-capabilities')
  await expect(capabilities).toHaveAttribute('aria-label', '平台核心能力')
  await expect(capabilities).toContainText('多集群治理')
  await expect(capabilities).toContainText('证据优先')
  await expect(capabilities).toContainText('审计闭环')
  await expect(capabilities).not.toContainText(/12|186|99|实时概况/)

  const loginPage = page.locator('main.login-page')
  const viewport = page.viewportSize()
  const introBox = await page.locator('.login-intro').boundingBox()
  const cardBox = await page.locator('.login-card').boundingBox()
  expect(viewport).not.toBeNull()
  expect(introBox).not.toBeNull()
  expect(cardBox).not.toBeNull()
  expect(Math.abs(Math.round(introBox!.width) - viewport!.width)).toBeLessThanOrEqual(8)
  expect(Math.abs(Math.round(introBox!.height) - viewport!.height)).toBeLessThanOrEqual(8)
  expect(cardBox!.y + cardBox!.height).toBeLessThanOrEqual(viewport!.height + 1)
  if (viewport!.width <= 720) {
    await expect(page.locator('.login-visual')).toBeHidden()
  } else {
    await expect(page.locator('.login-visual')).toBeVisible()
  }

  const username = page.locator('#username')
  const password = page.locator('#password')
  await username.focus()
  await expect(loginPage).toHaveAttribute('data-auth-phase', 'username')
  await password.focus()
  await expect(loginPage).toHaveAttribute('data-auth-phase', 'password')

  await expect(password).toHaveAttribute('type', 'password')
  await page.getByRole('button', { name: '显示密码' }).click()
  await expect(password).toHaveAttribute('type', 'text')
})

test('Login canvas renders pixels and exposes its runtime state', async ({ page }) => {
  await mockAnonymousAuth(page)
  await page.goto('/login')
  const canvas = page.locator('canvas.particle-network')
  await expect(canvas).toHaveAttribute('data-particles', /^[1-9]\d*$/)
  await expect(canvas).toHaveAttribute('data-running', 'true')

  const hasVisiblePixels = await canvas.evaluate((element) => {
    const canvasElement = element as HTMLCanvasElement
    const context = canvasElement.getContext('2d')
    if (!context) return false
    const pixels = context.getImageData(0, 0, canvasElement.width, canvasElement.height).data
    const stride = Math.max(4, Math.floor(pixels.length / 12000 / 4) * 4)
    for (let index = 3; index < pixels.length; index += stride) {
      if (pixels[index] > 0) return true
    }
    return false
  })
  expect(hasVisiblePixels).toBe(true)
})

test('Login canvas renders a static frame for reduced motion', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await mockAnonymousAuth(page)
  await page.goto('/login')
  const canvas = page.locator('canvas.particle-network')
  await expect(canvas).toHaveAttribute('data-reduced-motion', 'false')
  await expect(canvas).toHaveAttribute('data-running', 'true')

  await page.emulateMedia({ reducedMotion: 'reduce' })
  await expect(canvas).toHaveAttribute('data-reduced-motion', 'true')
  await expect(canvas).toHaveAttribute('data-running', 'false')
  await expect(canvas).toHaveAttribute('data-particles', /^[1-9]\d*$/)

  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await expect(canvas).toHaveAttribute('data-reduced-motion', 'false')
  await expect(canvas).toHaveAttribute('data-running', 'true')
})

test('Login canvas pauses while hidden and responds to container resize', async ({ page }) => {
  await mockAnonymousAuth(page)
  await page.goto('/login')
  const canvas = page.locator('canvas.particle-network')
  await expect(canvas).toHaveAttribute('data-running', 'true')
  const initialWidth = await canvas.evaluate((element) => Number.parseFloat((element as HTMLCanvasElement).style.width))

  await page.setViewportSize({ width: 920, height: 720 })
  await expect.poll(async () => canvas.evaluate((element) => Number.parseFloat((element as HTMLCanvasElement).style.width))).not.toBe(initialWidth)

  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await expect(canvas).toHaveAttribute('data-running', 'false')

  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await expect(canvas).toHaveAttribute('data-running', 'true')
})

test('Successful login exposes the confirmation transition before routing', async ({ page }) => {
  await mockAnonymousAuth(page)
  await mockSuccessfulLogin(page)
  await page.goto('/login')
  await page.locator('#username').fill('playwright-admin')
  await page.locator('#password').fill('playwright-password')
  await page.getByRole('button', { name: /进入控制台/ }).click()

  await expect(page.locator('main.login-page')).toHaveAttribute('data-auth-phase', 'success')
  await expect(page.getByRole('button', { name: /认证通过/ })).toBeVisible()
  await expect(page).toHaveURL('/')
})

test('Sidebar navigates after successful login', async ({ page }) => {
  await mockAnonymousAuth(page)
  await mockSuccessfulLogin(page)
  await page.goto('/login')
  await page.locator('#username').fill('playwright-admin')
  await page.locator('#password').fill('playwright-password')
  await page.getByRole('button', { name: /进入控制台/ }).click()

  await expect(page).toHaveURL('/')
  await page.getByRole('button', { name: '集群', exact: true }).click()
  await expect(page).toHaveURL('/clusters')
  await expect(page.locator('h1')).toContainText('集群接入')

  await page.getByRole('button', { name: '事件中心', exact: true }).click()
  await expect(page).toHaveURL('/events')
  await expect(page.locator('h1')).toContainText('事件中心')
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

test('Sidebar collapse state persists and remains reversible', async ({ page }) => {
  await page.goto('/')
  await page.evaluate(() => window.localStorage.removeItem('aiops.sidebar.collapsed'))
  await page.reload()

  const shell = page.locator('.app-shell')
  await page.getByRole('button', { name: '收起侧边栏' }).click()
  await expect(shell).toHaveClass(/sidebar-collapsed/)
  await expect(page.locator('.brand-copy')).toBeHidden()
  await expect(page.locator('.nav-item span').first()).toBeHidden()

  await page.goto('/clusters')
  await expect(page).toHaveURL('/clusters')
  await expect(shell).toHaveClass(/sidebar-collapsed/)
  await expect(page.locator('header.topbar')).toHaveCount(1)
  await expect(page.locator('h1')).toContainText('集群接入')

  await page.reload()
  await expect(shell).toHaveClass(/sidebar-collapsed/)
  await page.getByRole('button', { name: '展开侧边栏' }).click()
  await expect(shell).not.toHaveClass(/sidebar-collapsed/)
  await expect.poll(() => page.evaluate(() => window.localStorage.getItem('aiops.sidebar.collapsed'))).toBe('0')
})

test('Route navigation never exposes a white viewport', async ({ page }) => {
  await page.goto('/')
  await page.evaluate(() => {
    const state = { gapDetected: false, whiteDetected: false, samples: 0, running: true }
    const browserWindow = window as typeof window & { __routePaintState?: typeof state }
    browserWindow.__routePaintState = state
    const startedAt = performance.now()
    const sample = () => {
      const viewport = document.querySelector<HTMLElement>('.route-viewport')
      state.samples += 1
      if (!viewport) {
        state.gapDetected = true
      } else {
        const channels = getComputedStyle(viewport).backgroundColor.match(/\d+/g)?.map(Number) ?? []
        if (channels.length >= 3 && channels[0] >= 248 && channels[1] >= 248 && channels[2] >= 248) {
          state.whiteDetected = true
        }
      }
      if (performance.now() - startedAt < 500) {
        requestAnimationFrame(sample)
      } else {
        state.running = false
      }
    }
    requestAnimationFrame(sample)
  })

  await page.getByRole('button', { name: '集群', exact: true }).click()
  await expect(page).toHaveURL('/clusters')
  await expect(page.locator('.app-shell').last()).toBeVisible()
  await expect.poll(() => page.evaluate(() => {
    return (window as typeof window & { __routePaintState?: { running: boolean } }).__routePaintState?.running
  })).toBe(false)

  const paintState = await page.evaluate(() => {
    return (window as typeof window & {
      __routePaintState?: { gapDetected: boolean; whiteDetected: boolean; samples: number }
    }).__routePaintState
  })
  expect(paintState?.gapDetected).toBe(false)
  expect(paintState?.whiteDetected).toBe(false)
  expect(paintState?.samples).toBeGreaterThan(1)
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
