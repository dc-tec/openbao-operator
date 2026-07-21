import {defineConfig} from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  fullyParallel: true,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [['list'], ['html', {open: 'never'}]] : 'list',
  use: {
    baseURL: 'http://127.0.0.1:4173/openbao-operator/',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'pnpm run build && pnpm run serve --host 127.0.0.1 --port 4173 --no-open',
    url: 'http://127.0.0.1:4173/openbao-operator/',
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
  },
});
