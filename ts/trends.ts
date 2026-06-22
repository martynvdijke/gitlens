interface SnapshotData {
    id: number;
    repo_id: number;
    repo_name: string;
    timestamp: string;
    feat_count: number;
    fix_count: number;
    docs_count: number;
    chore_count: number;
    other_commit_count: number;
    total_commits_fetched: number;
    release_count: number;
    avg_lead_time_hours: number;
    workflow_success_count: number;
    workflow_failure_count: number;
    workflow_status: string;
}

interface SnapshotsResponse {
    snapshots: SnapshotData[];
}

interface ChartInstances {
    [key: string]: any;
}

const trendChartInstances: ChartInstances = {};

function destroyTrendCharts(): void {
    Object.values(trendChartInstances).forEach(c => {
        try { c.destroy(); } catch (_e) { /* ignore */ }
    });
    for (const key in trendChartInstances) {
        delete trendChartInstances[key];
    }
}

function getSinceDate(range: string): Date | null {
    const now = new Date();
    switch (range) {
        case '24h': return new Date(now.getTime() - 24 * 60 * 60 * 1000);
        case '7d': return new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
        case '30d': return new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);
        case '90d': return new Date(now.getTime() - 90 * 24 * 60 * 60 * 1000);
        default: return null;
    }
}

function buildTrendsUrl(): string {
    const rangeSelect = document.getElementById('trends-range') as HTMLSelectElement | null;
    const repoSelect = document.getElementById('trends-repo') as HTMLSelectElement | null;
    const range = rangeSelect?.value || '7d';
    const repoId = repoSelect?.value || '';

    const params = new URLSearchParams();
    const since = getSinceDate(range);
    if (since) {
        params.set('since', since.toISOString());
    }
    if (repoId) {
        params.set('repo_id', repoId);
    }
    return '/metrics/history?' + params.toString();
}

export function loadTrendsData(): void {
    const url = buildTrendsUrl();
    fetch(url)
        .then(r => r.json())
        .then((data: SnapshotsResponse) => {
            destroyTrendCharts();
            buildCommitTrendChart(data.snapshots);
            buildWorkflowTrendChart(data.snapshots);
            buildLeadTimeTrendChart(data.snapshots);
            buildReleaseTrendChart(data.snapshots);
        })
        .catch((e: Error) => console.error('Failed to load trends data:', e));
}

function buildCommitTrendChart(snapshots: SnapshotData[]): void {
    const ctx = document.getElementById('commitTrendChart') as HTMLCanvasElement | null;
    if (!ctx) return;

    const sorted = [...snapshots].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
    const labels = sorted.map(s => new Date(s.timestamp));

    trendChartInstances.commit = new (window as any).Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                {
                    label: 'Features',
                    data: sorted.map(s => s.feat_count),
                    backgroundColor: '#3fb950',
                    borderColor: '#3fb950',
                    fill: true,
                    tension: 0.1,
                    pointRadius: 1,
                },
                {
                    label: 'Fixes',
                    data: sorted.map(s => s.fix_count),
                    backgroundColor: '#f85149',
                    borderColor: '#f85149',
                    fill: true,
                    tension: 0.1,
                    pointRadius: 1,
                },
                {
                    label: 'Docs',
                    data: sorted.map(s => s.docs_count),
                    backgroundColor: '#d29922',
                    borderColor: '#d29922',
                    fill: true,
                    tension: 0.1,
                    pointRadius: 1,
                },
                {
                    label: 'Chore',
                    data: sorted.map(s => s.chore_count),
                    backgroundColor: '#8b949e',
                    borderColor: '#8b949e',
                    fill: true,
                    tension: 0.1,
                    pointRadius: 1,
                },
                {
                    label: 'Other',
                    data: sorted.map(s => s.other_commit_count),
                    backgroundColor: '#6e7681',
                    borderColor: '#6e7681',
                    fill: true,
                    tension: 0.1,
                    pointRadius: 1,
                },
            ],
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            interaction: {
                mode: 'index',
                intersect: false,
            },
            plugins: {
                legend: {
                    labels: { color: '#8b949e', boxWidth: 12 },
                },
            },
            scales: {
                x: {
                    type: 'time',
                    time: {
                        tooltipFormat: 'MMM d, yyyy HH:mm',
                    },
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e', maxTicksLimit: 10 },
                } as any,
                y: {
                    beginAtZero: true,
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e' },
                },
            },
        },
    });
}

function buildWorkflowTrendChart(snapshots: SnapshotData[]): void {
    const ctx = document.getElementById('workflowTrendChart') as HTMLCanvasElement | null;
    if (!ctx) return;

    const sorted = [...snapshots].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
    const labels = sorted.map(s => new Date(s.timestamp));

    const passRate = sorted.map(s => {
        const total = s.workflow_success_count + s.workflow_failure_count;
        return total > 0 ? (s.workflow_success_count / total) * 100 : null;
    });

    trendChartInstances.workflow = new (window as any).Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{
                label: 'Pass Rate (%)',
                data: passRate,
                backgroundColor: '#3fb950',
                borderColor: '#3fb950',
                fill: false,
                tension: 0.1,
                pointRadius: 2,
                spanGaps: true,
            }],
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    labels: { color: '#8b949e' },
                },
            },
            scales: {
                x: {
                    type: 'time',
                    time: {
                        tooltipFormat: 'MMM d, yyyy HH:mm',
                    },
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e', maxTicksLimit: 8 },
                } as any,
                y: {
                    min: 0,
                    max: 100,
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e', callback: (v: any) => v + '%' },
                },
            },
        },
    });
}

function buildLeadTimeTrendChart(snapshots: SnapshotData[]): void {
    const ctx = document.getElementById('leadTimeTrendChart') as HTMLCanvasElement | null;
    if (!ctx) return;

    const sorted = [...snapshots].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
    const labels = sorted.map(s => new Date(s.timestamp));

    trendChartInstances.leadTime = new (window as any).Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{
                label: 'Avg Lead Time (h)',
                data: sorted.map(s => s.avg_lead_time_hours),
                backgroundColor: '#58a6ff',
                borderColor: '#58a6ff',
                fill: false,
                tension: 0.1,
                pointRadius: 2,
                spanGaps: true,
            }],
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    labels: { color: '#8b949e' },
                },
            },
            scales: {
                x: {
                    type: 'time',
                    time: {
                        tooltipFormat: 'MMM d, yyyy HH:mm',
                    },
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e', maxTicksLimit: 8 },
                } as any,
                y: {
                    beginAtZero: true,
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e', callback: (v: any) => v + 'h' },
                },
            },
        },
    });
}

function buildReleaseTrendChart(snapshots: SnapshotData[]): void {
    const ctx = document.getElementById('releaseTrendChart') as HTMLCanvasElement | null;
    if (!ctx) return;

    // Group releases by day
    const dayBuckets: Record<string, number> = {};
    for (const s of snapshots) {
        const day = s.timestamp.substring(0, 10);
        dayBuckets[day] = (dayBuckets[day] || 0) + s.release_count;
    }

    const days = Object.keys(dayBuckets).sort();

    trendChartInstances.release = new (window as any).Chart(ctx, {
        type: 'bar',
        data: {
            labels: days.map(d => new Date(d)),
            datasets: [{
                label: 'Releases',
                data: days.map(d => dayBuckets[d]),
                backgroundColor: '#58a6ff',
                borderColor: '#58a6ff',
                borderWidth: 1,
            }],
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    labels: { color: '#8b949e' },
                },
            },
            scales: {
                x: {
                    type: 'time',
                    time: {
                        unit: 'day',
                        tooltipFormat: 'MMM d, yyyy',
                    },
                    grid: { color: '#21262d' },
                    ticks: { color: '#8b949e', maxTicksLimit: 12 },
                } as any,
                y: {
                    beginAtZero: true,
                    ticks: {
                        stepSize: 1,
                        color: '#8b949e',
                    },
                    grid: { color: '#21262d' },
                },
            },
        },
    });
}
