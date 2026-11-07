// @ts-check
const { defineConfig, devices } = require('@playwright/test');

// Testene kører mod en kørende app. Lokalt og i CI startes den med `docker compose up`
// (port 8080). BASE_URL kan sættes for at teste et andet sted.
module.exports = defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8080',
    trace: 'on-first-retry',
  },
  // Kun Chromium, så CI er hurtig. Firefox og WebKit kan tilføjes her.
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
