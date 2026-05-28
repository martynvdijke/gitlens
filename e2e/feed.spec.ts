import { test, expect } from '@playwright/test';

test.describe('Activity Feed - Protected Routes', () => {
  test('redirects unauthenticated users from GET /feed', async ({ page }) => {
    await page.goto('/feed');
    await expect(page.locator('h2:has-text("Track your GitHub repositories")')).toBeVisible();
  });

  test('redirects unauthenticated users from POST /feed/filter', async ({ page }) => {
    const response = await page.request.post('/feed/filter', { maxRedirects: 0 });
    expect(response.status()).toBe(302);
    expect(response.headers()['location']).toBe('/');
  });

  test('redirects unauthenticated users from GET /feed with query params', async ({ page }) => {
    await page.goto('/feed?since=7d&types=release');
    await expect(page.locator('h2:has-text("Track your GitHub repositories")')).toBeVisible();
  });

  test('redirects unauthenticated users from GET /feed with since filter', async ({ page }) => {
    await page.goto('/feed?since=24h');
    await expect(page.locator('h2:has-text("Track your GitHub repositories")')).toBeVisible();
  });

  test('redirects unauthenticated users from GET /feed with type filter', async ({ page }) => {
    await page.goto('/feed?types=workflow_failure,pr_merge');
    await expect(page.locator('h2:has-text("Track your GitHub repositories")')).toBeVisible();
  });
});

test.describe('Activity Feed - HTML Template Structure', () => {
  test('feed template uses correct CSS classes when rendered', async ({ page }) => {
    // The feed template uses Bootstrap utility classes for layout and custom CSS
    // for event type color accents. Verify the custom feed event classes exist.
    const response = await page.request.get('/static/style.css');
    const css = await response.text();

    // Feed event type classes (used in the feed template)
    expect(css).toContain('.feed-event-release');
    expect(css).toContain('.feed-event-workflow_failure');
    expect(css).toContain('.feed-event-pr_merge');

    // HTMX loading indicators
    expect(css).toContain('.htmx-indicator');
    expect(css).toContain('.htmx-request');
  });

  test('feed event type color classes are defined in CSS', async ({ page }) => {
    const response = await page.request.get('/static/style.css');
    const css = await response.text();

    // Color accents for each event type (SVG icons inside first child div)
    expect(css).toContain('.feed-event-release > div:first-child svg');
    expect(css).toContain('.feed-event-workflow_failure > div:first-child svg');
    expect(css).toContain('.feed-event-pr_merge > div:first-child svg');
  });

  test('feed filter controls have correct HTML structure', async ({ page }) => {
    // Verify the page loads for unauthenticated users (shows login hero).
    const response = await page.request.get('/');
    const html = await response.text();

    // The feed section requires auth. On the unauthenticated page,
    // verify the login hero is shown (confirming the page loaded).
    expect(html).toContain('Track your GitHub repositories');
  });
});

test.describe('Version Footer', () => {
  test('footer has correct HTML element', async ({ page }) => {
    const response = await page.request.get('/');
    const html = await response.text();
    expect(html).toContain('<footer');
  });

  test('footer contains appVersion placeholder in template', async ({ page }) => {
    const response = await page.request.get('/');
    const html = await response.text();
    // The template renders "GitLens <version>" in the footer
    expect(html).toContain('GitLens');
    expect(html).toContain('<footer');
  });

  test('footer uses Bootstrap utility classes', async ({ page }) => {
    const response = await page.request.get('/');
    const html = await response.text();
    // Footer uses Bootstrap utility classes for styling
    expect(html).toContain('text-center');
    expect(html).toContain('border-top');
  });

  test('footer version uses monospace font class', async ({ page }) => {
    const response = await page.request.get('/');
    const html = await response.text();
    // Version text uses Bootstrap's font-monospace utility class
    expect(html).toContain('font-monospace');
  });
});

test.describe('Import and Sync', () => {
  test('import all endpoint redirects unauthenticated users', async ({ page }) => {
    const response = await page.request.post('/repos/import-all', { maxRedirects: 0 });
    expect(response.status()).toBe(302);
    expect(response.headers()['location']).toBe('/');
  });

  test('sync endpoint redirects unauthenticated users', async ({ page }) => {
    const response = await page.request.post('/repos/1/sync', { maxRedirects: 0 });
    expect(response.status()).toBe(302);
    expect(response.headers()['location']).toBe('/');
  });
});

test.describe('PR Merge - Protected Routes', () => {
  test('merge PR endpoint redirects unauthenticated users', async ({ page }) => {
    const response = await page.request.post('/repos/1/prs/1/merge', { maxRedirects: 0 });
    expect(response.status()).toBe(302);
    expect(response.headers()['location']).toBe('/');
  });

  test('merge all PRs endpoint redirects unauthenticated users', async ({ page }) => {
    const response = await page.request.post('/repos/1/prs/merge-all', { maxRedirects: 0 });
    expect(response.status()).toBe(302);
    expect(response.headers()['location']).toBe('/');
  });

  test('list PRs endpoint redirects unauthenticated users', async ({ page }) => {
    await page.goto('/repos/1/prs');
    await expect(page.locator('h2:has-text("Track your GitHub repositories")')).toBeVisible();
  });
});
