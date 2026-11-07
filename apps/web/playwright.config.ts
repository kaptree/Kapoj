import { defineConfig } from '@playwright/test'

const backendURL = process.env.E2E_API_URL ?? 'http://127.0.0.1:18080'
const baseURL = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  globalSetup: './e2e/global-setup.ts',
  reporter: [['list'], ['html', { outputFolder: 'playwright-report', open: 'never' }]],
  use: {
    baseURL,
    channel: process.env.E2E_BROWSER_CHANNEL ?? 'chrome',
    actionTimeout: 10_000,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: `KAPOJ_BACKEND_URL=${backendURL} corepack pnpm dev -- --host 127.0.0.1 --port 5173`,
        url: 'http://127.0.0.1:5173',
        reuseExistingServer: true,
        timeout: 120_000,
      },
})
