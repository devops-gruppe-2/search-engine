// @ts-check
const { test, expect } = require('@playwright/test');

// En bruger søger fra forsiden med de felter, simulatoren også bruger

test('front page shows the search field and button', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveTitle(/Who Knows/);
  await expect(page.locator('#search-input')).toBeVisible();
  await expect(page.locator('#search-button')).toBeVisible();
});

test('searching keeps the query and shows results or a message', async ({ page }) => {
  await page.goto('/');
  await page.locator('#search-input').fill('python');
  await page.locator('#search-button').click();

  await expect(page).toHaveURL(/[?&]q=python/);
  await expect(page.locator('#search-input')).toHaveValue('python');

  // Databasen i CI er tom, så der kan være enten resultater eller en besked
  const results = page.locator('.search-result-title');
  const noResults = page.getByText('No results found');
  await expect(results.first().or(noResults)).toBeVisible();
});
