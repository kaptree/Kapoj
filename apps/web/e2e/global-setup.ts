import { request } from '@playwright/test'

export default async function globalSetup() {
  const apiURL = process.env.E2E_API_URL ?? 'http://127.0.0.1:18080'
  const ctx = await request.newContext()
  try {
    const health = await ctx.get(`${apiURL}/health`)
    if (!health.ok()) {
      throw new Error(
        `E2E backend is not healthy at ${apiURL}/health (HTTP ${health.status()}). ` +
          'Start the server and database first, or set E2E_API_URL.',
      )
    }
  } finally {
    await ctx.dispose()
  }
}
