# Activity Feed — Design Spec

## Overview

A unified chronological timeline of important events across all tracked GitHub repositories, surfaced as a toggleable section on the main dashboard.

**Event types tracked:** Releases published, workflow failures, merged pull requests.

---

## Data Model

### New Ent Schema: `Event`

```go
// ent/schema/event.go
type Event struct {
    ent.Schema
}

func (Event) Fields() []ent.Field {
    return []ent.Field{
        field.Int("repo_id"),                          // FK to repository
        field.Enum("type").Values("release",
            "workflow_failure", "pr_merge"),
        field.String("title"),                         // "v1.2.3", "feat: add login", etc.
        field.String("url"),                           // Link to resource
        field.String("metadata").Optional(),           // JSON blob (author, conclusion, etc.)
        field.Time("timestamp"),                       // When the event happened upstream
        field.Time("created_at").Default(time.Now),
    }
}
```

- Indexed on `(repo_id, timestamp DESC)` for efficient feed queries
- Indexed on `(type, timestamp DESC)` for type-filtered queries

### Query Shape

```sql
SELECT * FROM events
WHERE timestamp >= $since
  AND type IN ($types)     -- configurable filter
ORDER BY timestamp DESC
LIMIT 50
```

---

## Data Population

### During Sync (`internal/sync/syncer.go`)

On each repo sync, the syncer already fetches:
- Latest release (tag, date, conclusion)
- Latest workflow runs (status, conclusion)
- Open PRs (but only `state=open`)

**New logic after each sync:**

1. **Release event** — Compare `latest_release_tag` with stored value. If new, write a `release` event with the release tag as title.
2. **Workflow failure event** — If `workflow_status` is `failure`/`cancelled` and differs from last known state, write a `workflow_failure` event.
3. **PR merge event** — Fetch recently updated closed PRs via `GET /repos/{owner}/{repo}/pulls?state=closed&sort=updated&direction=desc&per_page=10`. Compare with previously stored merged PR numbers (tracked in a new JSON field `tracked_merged_prs` on Repository, or via dedup against existing events). Any newly seen merged PR produces a `pr_merge` event.

**New GitHub client method needed:**
```go
func (c *Client) ListRecentlyMergedPRs(token, owner, repo string) ([]*PullRequest, error)
```
Calls `GET /repos/{owner}/{repo}/pulls?state=closed&sort=updated&direction=desc&per_page=10` and filters for `merged_at IS NOT NULL`.

**Deduplication:** Check if an event with the same `(repo_id, type, title, timestamp)` already exists before inserting. Event writes are idempotent.

### Via Webhook

The existing `POST /webhook/github` handler receives push events. On push:
- Trigger a sync for the pushed repo (already happening)
- The sync will detect and record any new events

---

## API / Routes

### `GET /feed`

Returns an HTMX partial (`feed.html`) containing the event list.

**Query params:**
| Param  | Default | Values |
|--------|---------|--------|
| `since` | `7d`    | `24h`, `7d`, `30d`, `all` |
| `types` | all     | Comma-separated: `release`, `workflow_failure`, `pr_merge` |

**Response:** Rendered `feed.html` template with event groups by date.

### `POST /feed/filter`

HTMX form post to update the time range and type filters. Returns the same partial.

---

## UI

### Dashboard Integration

- A toggle button below the charts section: **"Show Activity Feed"**
- On click, `hx-get="/feed?since=7d"` loads the feed partial into a new `#feed-section`
- Time range dropdown + type checkboxes at the top of the feed section
- Re-filter triggers `hx-post="/feed/filter"` with form data

### Feed Item Layout

```
[Date header: Today / Yesterday / May 20]
  ┌──────────────────────────────────────┐
  │ 🔖 v2.4.3    repo-owner/repo-name   │
  │ Released 2 hours ago                 │
  ├──────────────────────────────────────┤
  │ ❌ CI failed  repo-owner/repo-name   │
  │ main build #142 — 30 min ago         │
  ├──────────────────────────────────────┤
  │ 🔀 Merged #87  repo-owner/repo-name  │
  │ "feat: add user profiles" — 1h ago   │
  └──────────────────────────────────────┘
```

- Grouped by date with sticky headers
- Each event type has a distinct icon + color
- Click event title/URL opens in new tab
- Empty state: "No events in this period"

### CSS

Extend `static/style.css`:
- `.feed-section` — container
- `.feed-filters` — filter bar (time range select, type checkboxes)
- `.feed-date-header` — sticky date group header
- `.feed-event` — event row with icon + content
- `.feed-event.release` — green accent
- `.feed-event.workflow_failure` — red accent
- `.feed-event.pr_merge` — blue accent

### Metadata per event type

Each event stores a JSON `metadata` blob:

| Event type | metadata fields |
|---|---|
| `release` | `{"conclusion": "success"|"failure"|"unknown"}` |
| `workflow_failure` | `{"run_id": 123, "branch": "main"}` |
| `pr_merge` | `{"author": "user", "number": 87, "base_ref": "main"}` |

---

## Files to Create

| File | Purpose |
|------|---------|
| `ent/schema/event.go` | Event Ent schema definition |
| `internal/handlers/feed.go` | Feed handler (GET /feed, POST /feed/filter) |
| `internal/handlers/feed_test.go` | Tests for feed handler |
| `static/feed.html` (or inline in index.html) | Feed HTMX partial template |

## Files to Modify

| File | Change |
|------|--------|
| `internal/github/client.go` | Add `ListRecentlyMergedPRs()` method |
| `internal/sync/syncer.go` | Add `detectAndRecordEvents()` after each repo sync |
| `main.go` | Register new routes, run Ent migration |
| `static/index.html` | Add feed toggle button + `#feed-section` container |
| `static/style.css` | Feed styling classes |

---

## Success Criteria

1. After importing repos and syncing, clicking "Show Activity Feed" shows recent releases, workflow failures, and merged PRs in chronological order
2. Time range filter (24h / 7d / 30d / All) correctly scopes results
3. Type filter (checkboxes) correctly scopes results
4. New events appear after manual "Refresh" or automatic sync
5. Webhook-triggered syncs also produce events
6. Empty state shows when no events match
7. All new code has passing tests
