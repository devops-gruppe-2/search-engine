// @ts-check
const { test, expect } = require('@playwright/test');

// Weather-siden henter rigtige data fra Open-Meteo gennem vores backend

test('weather link opens the forecast page', async ({ page }) => {
  await page.goto('/');
  await page.locator('#nav-weather').click();

  await expect(page).toHaveURL(/\/weather$/);
  await expect(page.getByRole('heading', { name: 'Weather forecast' })).toBeVisible();

  // Hvis Open-Meteo ikke kan nås fra CI, skal siden stadig vise en forståelig besked
  const forecast = page.locator('#weather-forecast');
  const unavailable = page.getByText('Weather forecast is unavailable');
  await expect(forecast.or(unavailable)).toBeVisible();
});
