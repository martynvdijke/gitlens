import { test, expect } from '@playwright/test';

test.describe('Home page (unauthenticated)', () => {
  test('loads with correct title', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveTitle('GitLens');
  });

  test('displays login prompt', async ({ page }) => {
    await page.goto('/');
    const heroHeading = page.locator('h2:has-text("Track your GitHub repositories")');
    await expect(heroHeading).toBeVisible();
  });

  test('shows GitHub login button', async ({ page }) => {
    await page.goto('/');
    const loginButton = page.locator('a.btn-success[href="/auth/github"]');
    await expect(loginButton).toBeVisible();
    await expect(loginButton).toContainText('Login with GitHub');
    await expect(loginButton).toHaveAttribute('href', '/auth/github');
  });
});

test.describe('Static assets', () => {
  test('serves stylesheet', async ({ page }) => {
    const response = await page.goto('/static/style.css');
    expect(response?.status()).toBe(200);
    expect(response?.headers()['content-type']).toContain('text/css');
  });

  test('serves favicon or returns 404 gracefully', async ({ page }) => {
    const response = await page.request.get('/static/favicon.ico');
    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Navigation', () => {
  test('brand link navigates home', async ({ page }) => {
    await page.goto('/');
    const brandLink = page.locator('.navbar-brand');
    await expect(brandLink).toContainText('GitLens');
    await expect(brandLink).toHaveAttribute('href', '/');
  });

  test('login link redirects to GitHub auth', async ({ page }) => {
    await page.goto('/');
    const [response] = await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/auth/github')),
      page.click('a.btn-success[href="/auth/github"]'),
    ]);
    expect(response.status()).toBe(302);
    expect(response.headers()['location']).toMatch(/github\.com\/login\/oauth\/authorize/);
  });
});

test.describe('Protected routes', () => {
  test('redirects unauthenticated users from dashboard', async ({ page }) => {
    await page.goto('/dashboard');
    // AuthRequired middleware redirects to /, which shows the login hero
    await expect(page.locator('h2:has-text("Track your GitHub repositories")')).toBeVisible();
  });

  test('redirects unauthenticated users from settings', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.locator('h2:has-text("Track your GitHub repositories")')).toBeVisible();
  });
});
