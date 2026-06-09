# Investigation: Repos Tab Missing Repositories & Metrics Layout Issues

## Issue 1: Repos Tab Shows Only 1 Repository (WebSocket Overwrite Bug)

### Symptom
The initial front page (Repos tab) displays only 1 repository card (`martynvdijke/dnd`), while the Metrics tab's "Per-Repository Breakdown" correctly shows 5 repositories.

### Root Cause
The HTMX WebSocket extension on the repo grid is misconfigured. When background sync completes, the syncer broadcasts a **single** `repo_card` HTML fragment via WebSocket. HTMX's `ws-connect` replaces the **entire innerHTML** of `#repo-grid` with each incoming message, overwriting all previously rendered repo cards.

### Evidence

**File: `static/index.html:153-176`**
```html
<div id="repo-grid" hx-ext="ws" ws-connect="/ws">
<div class="row g-3">
    {{range .Repos}}
    {{template "repo_card" .}}
    {{else}}
    ...
    {{end}}
</div>
</div>
```

The `ws-connect` is on `#repo-grid`. HTMX ws extension swaps innerHTML by default.

**File: `internal/sync/syncer.go:367-376`**
```go
func (s *Syncer) broadcastUpdate(repo *ent.Repository, u *ent.User) {
    if s.tmpl == nil {
        return
    }
    var buf bytes.Buffer
    err := s.tmpl.ExecuteTemplate(&buf, "repo_card", repo)
    if err != nil {
        return
    }
    s.hub.BroadcastToUser(int64(u.ID), buf.Bytes())
}
```

The syncer broadcasts a single `repo_card` (one `<div class="col-12">...</div>`), not a full `repo_list`. When this arrives at the client, HTMX replaces `#repo-grid`'s content with just this one card.

### Why Metrics Tab Shows All 5
The Metrics tab (`/metrics`) renders `metrics_tab` template, which does **not** have a WebSocket connection. It performs a fresh database query on each HTMX request and renders all repos statically. There is no WebSocket overwrite happening there.

### Fix Required
Two possible approaches:

**Option A (Recommended): Change WebSocket swap target**
Wrap each repo card in an element with a unique ID, and configure the WebSocket broadcast to target that specific ID for swap (using `hx-swap-oob` or a custom ws message handler). The syncer should wrap the broadcast in an OOB swap element:

```html
<div id="repo-{{.ID}}" hx-swap-oob="outerHTML">
  {{template "repo_card" .}}
</div>
```

**Option B: Move ws-connect to individual cards**
Remove `ws-connect` from `#repo-grid` and instead have each repo card individually connect to WebSocket updates. More complex but cleaner separation.

**Option C: Broadcast full repo_list**
Change `broadcastUpdate` to render and broadcast the entire `repo_list` template instead of a single card. Less efficient but simplest fix.

---

## Issue 2: Weird Layout for Metrics Graphs

### Symptom
The metrics charts (Commit Type Distribution bar chart, Workflow Pass Rate donut chart, DORA Metrics Summary) have inconsistent sizing and poor responsive behavior. The donut chart appears much larger than the bar chart, and the summary SVG is tiny in a full-width container.

### Root Cause
All SVG charts in `internal/handlers/charts.go` have **hardcoded fixed dimensions**:

- `svgBarChart`: `width = 300`, `height = 200` (line 48-49)
- `svgPieChart`: `size = 200` (line 100)
- `svgMetricTrend`: `width = 300`, `height = 120` (line 174-175)

These explicit `width`/`height` attributes on the `<svg>` elements prevent responsive scaling. The charts are placed in Bootstrap `col-md-6` and `col-12` containers, but the SVGs don't fill their parent containers.

### Evidence

**File: `internal/handlers/charts.go:47-97`**
```go
func svgBarChart(metrics *DORAMetrics) string {
    const width = 300
    const height = 200
    // ...
    s := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
        width, chartHeight, width, chartHeight)
```

**File: `internal/handlers/charts.go:99-171`**
```go
func svgPieChart(metrics *DORAMetrics) string {
    const size = 200
    // ...
    return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
        size, size+40, size, size+40)
```

**File: `internal/handlers/charts.go:173-193`**
```go
func svgMetricTrend(metrics *DORAMetrics) string {
    const width = 300
    const height = 120
    // ...
    return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
        width, height, width, height)
```

### Fix Required

**Short-term fix:**
Remove explicit `width`/`height` attributes from SVG elements and use CSS `width: 100%; height: auto;` to make them responsive. Keep `viewBox` for scaling. Update the `viewBox` dimensions to be consistent.

**Long-term fix (user-requested):**
Replace the hand-rolled SVG chart generation with a proper charting library (e.g., Chart.js, ApexCharts, or D3.js) that supports:
- Responsive sizing out of the box
- Interactive time range selection
- Better visual polish
- Hover tooltips and legends

The charts template in `static/index.html:418-436` would need to be updated to load the chart library and render charts from JSON data instead of inline SVG.

---

## Files Involved

| Issue | File | Relevance |
|-------|------|-----------|
| WebSocket overwrite | `static/index.html:153` | `repo-grid` has `ws-connect` |
| WebSocket overwrite | `internal/sync/syncer.go:367-376` | `broadcastUpdate` sends single card |
| WebSocket overwrite | `internal/ws/hub.go` | WebSocket broadcast mechanism |
| Chart layout | `internal/handlers/charts.go` | SVG generation with fixed dimensions |
| Chart layout | `static/index.html:418-436` | Charts template |
| Chart layout | `static/index.html:554-673` | Metrics tab template |

## Recommended Fix Priority
1. **Critical**: Fix WebSocket overwrite bug (Issue 1) - this is a functional bug affecting core UX
2. **Medium**: Fix SVG responsive sizing (Issue 2 short-term)
3. **Enhancement**: Replace with chart library (Issue 2 long-term - user requested)
