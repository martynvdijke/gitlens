import { toggleTheme, initTheme } from './theme.js';
import { loadChartData } from './charts.js';
import { loadTrendsData } from './trends.js';
import { loadYearOverviewData } from './year-overview.js';
import { forgejoLogin } from './forgejo.js';
import { copyToClipboard } from './clipboard.js';

// Expose functions globally for HTML onclick/onchange handlers
(window as any).toggleTheme = toggleTheme;
(window as any).forgejoLogin = forgejoLogin;
(window as any).copyToClipboard = copyToClipboard;
(window as any).loadChartData = loadChartData;
(window as any).loadTrendsData = loadTrendsData;
(window as any).loadYearOverviewData = loadYearOverviewData;

// Initialize theme and tooltips on DOMContentLoaded
document.addEventListener('DOMContentLoaded', () => {
    initTheme();
    // Initialize Bootstrap tooltips
    const tooltipTriggerList = document.querySelectorAll<HTMLElement>('[data-bs-toggle="tooltip"]');
    if (tooltipTriggerList.length > 0 && (window as any).bootstrap?.Tooltip) {
        tooltipTriggerList.forEach(el => new (window as any).bootstrap.Tooltip(el));
    }
    // Check for changelog banner
    initChangelogBanner();
});

// Auto-init charts when metrics content is loaded via HTMX
document.addEventListener('htmx:afterSwap', (e: Event) => {
    const detail = (e as CustomEvent).detail;
    const target = detail?.target as HTMLElement | null;
    if (target && target.querySelector('[data-chart-init]')) {
        loadChartData('');
    }
    if (target && target.querySelector('[data-trends-init]')) {
        loadTrendsData();
    }
    if (target && target.querySelector('[data-year-overview-init]')) {
        loadYearOverviewData();
    }
});

// Auto-init trends when dropdowns change (delegated event listener)
document.addEventListener('change', (e: Event) => {
    const el = e.target as HTMLElement;
    if (el.id === 'trends-range' || el.id === 'trends-repo') {
        loadTrendsData();
    }
    if (el.id === 'year-select' || el.id === 'year-repo') {
        loadYearOverviewData();
    }
});

// Import progress bar
let importInterval: ReturnType<typeof setInterval> | null = null;

function updateImportProgress(): void {
    fetch('/repos/import-progress')
        .then(r => r.json())
        .then((data: { total: number; current: number }) => {
            const bar = document.getElementById('import-progress-bar');
            const fill = document.getElementById('import-progress-fill');
            const text = document.getElementById('import-progress-text');
            if (!bar || !fill || !text) return;

            if (data.total === 0) {
                bar.style.display = 'none';
                if (importInterval) { clearInterval(importInterval); importInterval = null; }
                return;
            }

            bar.style.display = 'block';
            const pct = Math.min(100, Math.round((data.current / data.total) * 100));
            fill.style.width = pct + '%';
            fill.setAttribute('aria-valuenow', String(pct));
            text.textContent = pct + '% (' + data.current + '/' + data.total + ')';

            if (data.current >= data.total) {
                setTimeout(() => { bar.style.display = 'none'; }, 3000);
                if (importInterval) { clearInterval(importInterval); importInterval = null; }
            }
        })
        .catch(() => { /* ignore polling errors */ });
}

(window as any).startImportProgress = function startImportProgress(): void {
    if (importInterval) clearInterval(importInterval);
    updateImportProgress();
    importInterval = setInterval(updateImportProgress, 2000);
};

// Changelog version banner
function initChangelogBanner(): void {
    const banner = document.getElementById('changelog-banner');
    const versionEl = document.getElementById('app-version');
    if (!banner || !versionEl) return;

    const currentVer = versionEl.getAttribute('data-version') || '';
    if (!currentVer) return;

    const storedVer = localStorage.getItem('gitlens-changelog-seen');
    if (storedVer === currentVer) { banner.style.display = 'none'; return; }

    banner.style.display = 'block';
    const dismissBtn = banner.querySelector('.btn-close');
    if (dismissBtn) {
        dismissBtn.addEventListener('click', () => {
            localStorage.setItem('gitlens-changelog-seen', currentVer);
            banner.style.display = 'none';
        });
    }
}

// Pull-to-refresh gesture (mobile)
(function initPullToRefresh(): void {
    let startY = 0;
    let pulling = false;
    const threshold = 50;
    const container = document.documentElement;
    let indicator: HTMLDivElement | null = null;

    function getIndicator(): HTMLDivElement {
        if (!indicator) {
            indicator = document.createElement('div');
            indicator.id = 'ptr-indicator';
            indicator.style.cssText =
                'position:fixed;top:0;left:0;right:0;height:3px;background:var(--accent-info,#58a6ff);z-index:9999;transform:scaleX(0);transform-origin:left;transition:transform 0.2s ease;';
            document.body.appendChild(indicator);
        }
        return indicator;
    }

    container.addEventListener('touchstart', (e: TouchEvent) => {
        if (window.scrollY <= 0) {
            startY = e.touches[0].clientY;
            pulling = true;
        }
    }, { passive: true });

    container.addEventListener('touchmove', (e: TouchEvent) => {
        if (!pulling) return;
        const dy = e.touches[0].clientY - startY;
        if (dy > 0 && window.scrollY <= 0) {
            const progress = Math.min(dy / threshold, 1);
            const ind = getIndicator();
            ind.style.transform = `scaleX(${progress})`;
        }
    }, { passive: true });

    container.addEventListener('touchend', (_e: TouchEvent) => {
        if (!pulling) return;
        pulling = false;
        const dy = _e.changedTouches[0].clientY - startY;
        const ind = getIndicator();
        ind.style.transform = 'scaleX(0)';

        if (dy >= threshold && window.scrollY <= 0) {
            const h = (window as any).htmx;
            if (!h) return;
            // Trigger refresh on current view
            const mainContent = document.getElementById('main-content');
            const currentUrl = mainContent?.getAttribute('hx-get') || '/dashboard';
            h.trigger('body', 'htmx:refresh');
            // Attempt to reload current HTMX view
            const activeTab = document.querySelector('#tab-bar .nav-link.active, #bottom-nav .nav-link.active');
            if (activeTab) {
                const url = activeTab.getAttribute('href') || '/dashboard';
                h.ajax('GET', url, { target: '#main-content', swap: 'innerHTML' });
            } else {
                h.ajax('GET', currentUrl, { target: '#main-content', swap: 'innerHTML' });
            }
        }
    }, { passive: true });
})();
