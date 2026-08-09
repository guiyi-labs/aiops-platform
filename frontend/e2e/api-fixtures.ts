import type { Page, Route } from '@playwright/test'

const authenticatedSession = {
  access_token: 'playwright-access-token',
  token_type: 'Bearer',
  expires_in: 900,
  user: {
    id: 1,
    username: 'playwright-admin',
    display_name: 'Playwright Admin',
    roles: ['system_admin'],
  },
}

const anonymousSession = {
  access_token: '',
  token_type: 'Bearer',
  expires_in: 0,
  user: null,
}

const emptyAPIResponse = {
  status: 'ready',
  items: [],
  findings: [],
  recent: [],
  failures: [],
  total: 0,
  open: 0,
  confirmed: 0,
  resolved: 0,
  dismissed: 0,
  overdue: 0,
  remaining: 0,
  truncated: false,
}

function fulfillJSON(route: Route, body: unknown) {
  return route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(body),
  })
}

export async function mockAuthenticatedAPI(page: Page) {
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/auth/refresh') {
      await fulfillJSON(route, authenticatedSession)
      return
    }
    await fulfillJSON(route, emptyAPIResponse)
  })
}

export async function mockAnonymousAuth(page: Page) {
  await page.route('**/api/v1/auth/refresh', (route) => fulfillJSON(route, anonymousSession))
}
