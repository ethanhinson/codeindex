import { defineConfig, devices } from '@playwright/test'

const PORT = 7799
const BASE = `http://127.0.0.1:${PORT}`

// Builds the Go binary (which embeds the current internal/webserver/dist)
// and serves this repo so the browser hits the real read model. Build the
// SPA (`npm run build`) before running so dist is current.
export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL: BASE,
    screenshot: 'only-on-failure',
    trace: 'on-first-retry',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command:
      'cd .. && go build -o /tmp/lore-graph-serve ./cmd/codeindex && exec /tmp/lore-graph-serve serve "$(pwd)" --addr 127.0.0.1:7799',
    url: `${BASE}/api/health`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
})
