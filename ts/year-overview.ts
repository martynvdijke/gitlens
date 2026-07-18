interface YearStatsResponse {
    year: number;
    total_commits: number;
    active_days: number;
    most_active_day: { date: string; count: number } | null;
    longest_streak: number;
    current_streak: number;
    busiest_weekday: string;
    monthly_totals: number[];
    daily_counts: Record<string, number>;
    top_repos: { repo_id: number; full_name: string; commits: number }[];
    repo_id?: number;
    repo_name?: string;
}

// Color scale for heatmap (GitHub-like greens)
const HEATMAP_COLORS = ['#0d1117', '#0e4429', '#006d32', '#26a641', '#39d353'];

let yearChartInstance: any = null;

function buildYearUrl(): string {
    const yearSelect = document.getElementById('year-select') as HTMLSelectElement | null;
    const repoSelect = document.getElementById('year-repo') as HTMLSelectElement | null;
    const year = yearSelect?.value || String(new Date().getFullYear());
    const repoId = repoSelect?.value || '';

    const params = new URLSearchParams();
    params.set('year', year);
    if (repoId) {
        params.set('repo_id', repoId);
    }
    return '/year-overview/stats?' + params.toString();
}

function getQuartiles(counts: number[]): [number, number, number] {
    if (counts.length === 0) return [0, 0, 0];
    const sorted = [...counts].sort((a, b) => a - b);
    const len = sorted.length;
    return [
        sorted[Math.floor(len * 0.25)],   // Q1
        sorted[Math.floor(len * 0.5)],    // Q2
        sorted[Math.floor(len * 0.75)],   // Q3
    ];
}

function getHeatmapColor(count: number, q1: number, q2: number, q3: number): string {
    if (count === 0) return HEATMAP_COLORS[0];
    if (count <= q1) return HEATMAP_COLORS[1];
    if (count <= q2) return HEATMAP_COLORS[2];
    if (count <= q3) return HEATMAP_COLORS[3];
    return HEATMAP_COLORS[4];
}

function renderHeatmap(dailyCounts: Record<string, number>, year: number): void {
    const container = document.getElementById('year-heatmap');
    if (!container) return;

    const counts = Object.values(dailyCounts).filter(c => c > 0);
    const [q1, q2, q3] = getQuartiles(counts.length > 0 ? counts : [0]);

    // Build a dense day map
    const dayMap: Record<string, number> = {};
    for (const [day, count] of Object.entries(dailyCounts)) {
        dayMap[day] = count;
    }

    // Create a grid for the heatmap
    const startDate = new Date(Date.UTC(year, 0, 1));
    const endDate = new Date(Date.UTC(year, 11, 31));

    // Determine starting day of week (0=Sun) - GitHub's heatmap starts on Sunday
    const startDayOfWeek = startDate.getUTCDay();

    // Calculate total days
    const msPerDay = 86400000;
    const totalDays = Math.round((endDate.getTime() - startDate.getTime()) / msPerDay) + 1;

    // Generate all days in the year
    const days: { date: string; count: number; dayOfWeek: number; week: number }[] = [];
    for (let i = 0; i < totalDays; i++) {
        const d = new Date(startDate.getTime() + i * msPerDay);
        const dateStr = d.toISOString().substring(0, 10);
        const count = dayMap[dateStr] || 0;
        const dayOfWeek = d.getUTCDay();
        const dayOffset = i + startDayOfWeek;
        const week = Math.floor(dayOffset / 7);
        days.push({ date: dateStr, count, dayOfWeek, week });
    }

    if (days.length === 0) {
        container.innerHTML = '<div class="text-secondary small text-center py-4">No activity data for this year.</div>';
        return;
    }

    const totalWeeks = days.length > 0 ? days[days.length - 1].week + 1 : 53;

    // Build HTML table-like grid
    let html = '<div style="display:grid;grid-template-columns:repeat(' + totalWeeks + ',12px);gap:3px;padding:4px">';

    for (let w = 0; w < totalWeeks; w++) {
        for (let dow = 0; dow < 7; dow++) {
            const day = days.find(d => d.week === w && d.dayOfWeek === dow);
            if (day) {
                const color = getHeatmapColor(day.count, q1, q2, q3);
                html += '<div style="width:12px;height:12px;border-radius:2px;background:' + color + '" '
                    + 'title="' + day.date + ': ' + day.count + ' commit' + (day.count !== 1 ? 's' : '') + '" '
                    + 'aria-label="' + day.date + ': ' + day.count + ' commit' + (day.count !== 1 ? 's' : '') + '">'
                    + '</div>';
            } else {
                html += '<div style="width:12px;height:12px"></div>';
            }
        }
    }
    html += '</div>';

    // Add legend
    html += '<div class="d-flex align-items-center gap-1 mt-2 justify-content-end small text-secondary">';
    html += '<span>Less</span>';
    for (const color of HEATMAP_COLORS) {
        html += '<span style="display:inline-block;width:10px;height:10px;border-radius:2px;background:' + color + '"></span>';
    }
    html += '<span>More</span></div>';

    container.innerHTML = html;
}

function renderMonthlyChart(monthlyTotals: number[]): void {
    const ctx = document.getElementById('yearMonthlyChart') as HTMLCanvasElement | null;
    if (!ctx) return;

    // Destroy previous chart
    if (yearChartInstance) {
        try { yearChartInstance.destroy(); } catch (_e) { /* ignore */ }
    }

    const labels = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];

    yearChartInstance = new (window as any).Chart(ctx, {
        type: 'bar',
        data: {
            labels: labels,
            datasets: [{
                label: 'Commits',
                data: monthlyTotals,
                backgroundColor: '#26a641',
                borderColor: '#26a641',
                borderWidth: 1,
                borderRadius: 3,
            }],
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: { display: false },
            },
            scales: {
                x: {
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e' },
                },
                y: {
                    beginAtZero: true,
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e' },
                },
            },
        },
    });
}

function renderTopRepos(topRepos: { repo_id: number; full_name: string; commits: number }[]): void {
    const container = document.getElementById('year-top-repos');
    if (!container) return;

    if (topRepos.length === 0) {
        container.innerHTML = '<div class="text-secondary small">No data yet.</div>';
        return;
    }

    let html = '<div class="d-flex flex-column gap-1">';
    for (const repo of topRepos) {
        html += '<div class="d-flex align-items-center justify-content-between py-1 px-2 bg-dark rounded border border-secondary">'
            + '<span class="small text-light">' + repo.full_name + '</span>'
            + '<span class="small fw-semibold text-info">' + repo.commits + ' commits</span>'
            + '</div>';
    }
    html += '</div>';
    container.innerHTML = html;
}

export function loadYearOverviewData(): void {
    const url = buildYearUrl();

    // Show loading
    const loading = document.getElementById('year-loading');
    if (loading) loading.style.display = 'block';

    fetch(url)
        .then(r => r.json())
        .then((data: YearStatsResponse) => {
            // Hide loading
            if (loading) loading.style.display = 'none';

            // Update stat cards
            setTextContent('year-total-commits', formatNumber(data.total_commits));
            setTextContent('year-active-days', String(data.active_days));
            setTextContent('year-longest-streak', data.longest_streak > 0 ? data.longest_streak + ' days' : '—');
            setTextContent('year-current-streak', data.current_streak > 0 ? data.current_streak + ' days' : '—');
            setTextContent('year-busiest-weekday', data.busiest_weekday || '—');
            setTextContent('year-most-active-day',
                data.most_active_day
                    ? data.most_active_day.date + ' (' + data.most_active_day.count + ')'
                    : '—'
            );

            // Render heatmap
            renderHeatmap(data.daily_counts, data.year);

            // Render monthly chart
            renderMonthlyChart(data.monthly_totals);

            // Render top repos
            renderTopRepos(data.top_repos);
        })
        .catch((e: Error) => {
            if (loading) loading.style.display = 'none';
            console.error('Failed to load year overview data:', e);
        });
}

function formatNumber(n: number): string {
    if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
    if (n >= 1000) return (n / 1000).toFixed(1) + 'k';
    return String(n);
}

function setTextContent(id: string, text: string): void {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
}
