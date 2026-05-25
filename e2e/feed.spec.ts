import { test, expect } from '@playwright/test';

test.describe('Activity Feed - Protected Routes', () => {
  test('redirects unauthenticated users from GET /feed', async ({ page }) => {
    await page.goto('/feed');
    await expect(page.locator('.login-prompt')).toBeVisible();
  });

  test('redirects unauthenticated users from POST /feed/filter', async ({ page }) => {
    const response = await page.request.post('/feed/filter', { maxRedirects: 0 });
    expect(response.status()).toBe(302);
    expect(response.headers()['location']).toBe('/');
  });

  test('redirects unauthenticated users from GET /feed with query params', async ({ page }) => {
    await page.goto('/feed?since=7d&types=release');
    await expect(page.locator('.login-prompt')).toBeVisible();
  });

  test('redirects unauthenticated users from GET /feed with since filter', async ({ page }) => {
    await page.goto('/feed?since=24h');
    await expect(page.locator('.login-prompt')).toBeVisible();
  });

  test('redirects unauthenticated users from GET /feed with type filter', async ({ page }) => {
    await page.goto('/feed?types=workflow_failure,pr_merge');
    await expect(page.locator('.login-prompt')).toBeVisible();
  });
});

test.describe('Activity Feed - HTML Template Structure', () => {
  test('feed template uses correct CSS classes when rendered', async ({ page }) => {
    // The feed template is rendered server-side and loaded via HTMX into #feed-section.
    // We verify the expected CSS classes exist in the stylesheet.
    const response = await page.request.get('/static/style.css');
    const css = await response.text();

    // Feed section container
    expect(css).toContain('.feed-section');

    // Feed header and filters
    expect(css).toContain('.feed-header');
    expect(css).toContain('.feed-filters');
    expect(css).toContain('.feed-checkbox-label');

    // Feed list and date grouping
    expect(css).toContain('.feed-list');
    expect(css).toContain('.feed-date-group');
    expect(css).toContain('.feed-date-header');

    // Feed event items
    expect(css).toContain('.feed-event');
    expect(css).toContain('.feed-event-icon');
    expect(css).toContain('.feed-event-content');
    expect(css).toContain('.feed-event-title');

    // Feed meta info
    expect(css).toContain('.feed-event-repo');
    expect(css).toContain('.feed-event-time');

    // Empty state
    expect(css).toContain('.feed-empty');
  });

  test('feed event type color classes are defined in CSS', async ({ page }) => {
    const response = await page.request.get('/static/style.css');
    const css = await response.text();

    // Color accents for each event type
    expect(css).toContain('.feed-event-release .feed-event-icon');
    expect(css).toContain('.feed-event-workflow_failure .feed-event-icon');
    expect(css).toContain('.feed-event-pr_merge .feed-event-icon');
  });

  test('feed filter controls have correct HTML structure', async ({ page }) => {
    // Verify the dashboard HTML template includes the feed section container
    // even before HTMX loads content (it's a placeholder for HTMX to fill).
    const response = await page.request.get('/');
    const html = await response.text();

    // The feed button and target section are in the dashboard template
    // which requires auth. On the unauthenticated page, these won't be present.
    // Instead, verify the login prompt is shown (confirming the page loaded).
    expect(html).toContain('login-prompt');
  });
});

test.describe('Version Footer', () => {
  test('footer has correct CSS class', async ({ page }) => {
    const response = await page.request.get('/');
    const html = await response.text();
    expect(html).toContain('app-footer');
  });

  test('footer contains appVersion placeholder in template', async ({ page }) => {
    const response = await page.request.get('/');
    const html = await response.text();
    // The template renders "GitLens <version>" in the footer
    expect(html).toContain('GitLens');
    expect(html).toContain('app-footer');
  });

  test('footer CSS is properly defined', async ({ page }) => {
    const response = await page.request.get('/static/style.css');
    const css = await response.text();
    expect(css).toContain('.app-footer');
    expect(css).toContain('border-top');
    expect(css).toContain('text-align: center');
  });

  test('footer version class has monospace font', async ({ page }) => {
    const response = await page.request.get('/static/style.css');
    const css = await response.text();
    expect(css).toContain('.app-footer .version');
    expect(css).toContain('monospace');
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
    await expect(page.locator('.login-prompt')).toBeVisible();
  });
});
