import { test, expect } from '@playwright/test';

test.describe('Landing page (unauthenticated)', () => {
  test('displays hero section with gradient title', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.hero-section')).toBeVisible();
    await expect(page.locator('.hero-section h2')).toHaveText(
      'Track your GitHub repositories'
    );
  });

  test('shows hero subtitle', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.hero-subtitle')).toBeVisible();
    await expect(page.locator('.hero-subtitle')).toHaveText(
      'Monitor latest commits, releases, and workflow status across all your projects in one unified dashboard.'
    );
  });

  test('shows GitHub login button with icon', async ({ page }) => {
    await page.goto('/');
    const loginButton = page.locator('.btn-github-large');
    await expect(loginButton).toBeVisible();
    await expect(loginButton).toContainText('Login with GitHub');
    await expect(loginButton).toHaveAttribute('href', '/auth/github');
  });

  test('displays three feature cards', async ({ page }) => {
    await page.goto('/');
    const featureCards = page.locator('.feature-card');
    await expect(featureCards).toHaveCount(3);
  });

  test('feature cards have correct headings', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.feature-card').nth(0).locator('h3')).toHaveText('Multi-Repo Dashboard');
    await expect(page.locator('.feature-card').nth(1).locator('h3')).toHaveText('DORA Metrics');
    await expect(page.locator('.feature-card').nth(2).locator('h3')).toHaveText('Live Monitoring');
  });

  test('login link redirects to GitHub auth', async ({ page }) => {
    await page.goto('/');
    const [response] = await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/auth/github')),
      page.click('.btn-github-large'),
    ]);
    expect(response.status()).toBe(302);
    expect(response.headers()['location']).toMatch(/github\.com\/login\/oauth\/authorize/);
  });
});

test.describe('Badge endpoint', () => {
  test('returns SVG for known repo (404 -> unknown badge)', async ({ page }) => {
    const response = await page.request.get('/badge/nonexistent/repo');
    expect(response.status()).toBe(404);
    const body = await response.text();
    expect(response.headers()['content-type']).toBe('image/svg+xml');
    expect(body).toContain('<svg');
    expect(body).toContain('unknown');
  });

  test('returns correct content type', async ({ page }) => {
    const response = await page.request.get('/badge/any/repo');
    expect(response.headers()['content-type']).toBe('image/svg+xml');
  });

  test('badge SVG has proper dimensions', async ({ page }) => {
    const response = await page.request.get('/badge/some/repo');
    const body = await response.text();
    expect(body).toContain('width="140"');
    expect(body).toContain('height="20"');
  });

  test('badge SVG has workflow label', async ({ page }) => {
    const response = await page.request.get('/badge/test/repo');
    const body = await response.text();
    expect(body).toContain('workflow');
  });

  test('badge handles special characters in repo name', async ({ page }) => {
    const response = await page.request.get('/badge/org-name/repo-name');
    expect(response.status()).toBe(404);
    const body = await response.text();
    expect(body).toContain('<svg');
  });
});

test.describe('SVG endpoint error handling', () => {
  test('badge with very long path', async ({ page }) => {
    const longOwner = 'a'.repeat(100);
    const response = await page.request.get(`/badge/${longOwner}/repo`);
    expect(response.status()).toBe(404);
    expect(response.headers()['content-type']).toBe('image/svg+xml');
  });

  test('badge with empty owner', async ({ page }) => {
    const response = await page.request.get('/badge//repo');
    expect(response.status()).toBe(404);
  });
});

test.describe('Protected routes', () => {
  test('redirects unauthenticated from charts', async ({ page }) => {
    await page.goto('/charts');
    await expect(page.locator('.login-prompt')).toBeVisible();
  });

  test('redirects unauthenticated from repos list', async ({ page }) => {
    await page.goto('/repos');
    await expect(page.locator('.login-prompt')).toBeVisible();
  });

  test('redirects unauthenticated from repo detail routes', async ({ page }) => {
    await page.goto('/repos/1/prs');
    await expect(page.locator('.login-prompt')).toBeVisible();
  });
});

test.describe('Static assets', () => {
  test('serves logo SVG', async ({ page }) => {
    const response = await page.request.get('/static/logo.svg');
    expect(response.status()).toBe(200);
    expect(response.headers()['content-type']).toContain('image/svg+xml');
  });
});
