import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  use: {
    baseURL: process.env.CRYOL_E2E_URL ?? 'http://127.0.0.1:18081',
    launchOptions: { executablePath: process.env.CRYOL_E2E_BROWSER },
    viewport: { width: 1440, height: 900 },
  },
})
