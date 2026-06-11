import { defineConfig, devices } from '@playwright/test';

const port = Number(process.env.CITYEVENTS_FRONTEND_PORT || 18088);
const baseURL = process.env.CITYEVENTS_E2E_BASE_URL || `http://127.0.0.1:${port}`;
const skipWebServer = process.env.CITYEVENTS_E2E_SKIP_WEBSERVER === 'true';

export default defineConfig({
  testDir: './e2e',
  timeout: 90_000,
  expect: {
    timeout: 15_000,
  },
  fullyParallel: false,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: skipWebServer
    ? undefined
    : {
        command: `bash ../scripts/start-local.sh --frontend-port ${port}`,
        url: baseURL,
        reuseExistingServer: false,
        timeout: 180_000,
        stdout: 'pipe',
        stderr: 'pipe',
      },
});
