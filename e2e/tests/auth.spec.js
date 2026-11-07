// @ts-check
const { test, expect } = require('@playwright/test');

// Hver test laver sin egen bruger, så testene ikke afhænger af hinanden
function newUser() {
  const id = `${Date.now()}${Math.floor(Math.random() * 10000)}`;
  return { username: `e2e_${id}`, email: `e2e_${id}@example.com`, password: 'e2e-password-123' };
}

async function registerViaForm(page, user) {
  await page.goto('/register');
  await page.locator('input[name="username"]').fill(user.username);
  await page.locator('input[name="email"]').fill(user.email);
  await page.locator('input[name="password"]').fill(user.password);
  await page.locator('#register-button').click();
  await expect(page).toHaveURL(/\/login$/);
}

async function loginViaForm(page, username, password) {
  await page.locator('input[name="username"]').fill(username);
  await page.locator('input[name="password"]').fill(password);
  await page.locator('#login-button').click();
}

test('log in link on the front page opens the login page', async ({ page }) => {
  await page.goto('/');
  await page.locator('#nav-login').click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.locator('#login-button')).toBeVisible();
});

test('a new user can register, log in and log out', async ({ page }) => {
  const user = newUser();
  await registerViaForm(page, user);

  await loginViaForm(page, user.username, user.password);
  const logout = page.locator('#nav-logout');
  await expect(logout).toBeVisible();
  await expect(logout).toContainText(user.username);
  await expect(page.locator('#nav-login')).toHaveCount(0);

  await logout.click();
  await expect(page.locator('#nav-login')).toBeVisible();
  await expect(page.locator('#nav-logout')).toHaveCount(0);
});

test('logging in with a wrong password shows an error', async ({ page }) => {
  const user = newUser();
  await registerViaForm(page, user);

  await loginViaForm(page, user.username, 'wrong-password');
  await expect(page.getByText('Invalid username or password')).toBeVisible();
  await expect(page.locator('#nav-logout')).toHaveCount(0);
});
