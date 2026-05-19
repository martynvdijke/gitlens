# GitOverviewer

A Go-based web application for monitoring and visualizing GitHub repository activity across multiple repositories at a glance. Tracks commits, releases, workflow status, and DORA metrics.

## Features

- **Multi-repo dashboard** — View commits, releases, and CI status for all your GitHub repos in one place
- **Automated syncing** — Per-user configurable sync intervals (1 min to 24 hours)
- **DORA metrics** — Commit type breakdown (feat/fix/docs/chore), workflow pass rate, lead time, release frequency
- **GitHub webhook** — Real-time updates on push events
- **WebSocket live updates** — Repo cards refresh automatically when data changes
- **GitHub OAuth** — Secure authentication via GitHub

## Architecture

```
Browser (HTMX + WebSocket)
    |
Gin HTTP Server (:6270)
    |
    +-- Auth routes      --> GitHub OAuth
    +-- Dashboard routes --> HTML + HTMX partials
    +-- Settings routes  --> HTML forms
    +-- Webhook route    --> GitHub push events
    +-- WebSocket route  --> Real-time updates
    |
internal/github.Client --> GitHub REST API v3
    |
internal/sync.Syncer   --> Periodic + on-demand sync
    |
ent.Client (ORM)       --> SQLite
```

## Prerequisites

- Go 1.26+
- Node.js 24+ (for TypeScript compilation)
- C compiler (for CGO SQLite driver)
- A [GitHub OAuth App](https://github.com/settings/developers)

## Quick Start

### 1. Register a GitHub OAuth App

Create a new OAuth App at https://github.com/settings/developers with:

- **Homepage URL:** `http://localhost:6270`
- **Authorization callback URL:** `http://localhost:6270/auth/github/callback`

### 2. Configure environment variables

```bash
export GITHUB_CLIENT_ID=your_client_id
export GITHUB_CLIENT_SECRET=your_client_secret
export GITHUB_REDIRECT_URL=http://localhost:6270/auth/github/callback
export DB_PATH=gitoverviewer.db
export PORT=6270
```

### 3. Install dependencies

```bash
task install
```

This runs `go mod download`, `npm ci`, installs Playwright browsers, and compiles TypeScript.

### 4. Run

```bash
task run
```

Open http://localhost:6270 and log in with GitHub.

## Configuration

| Environment Variable | Default | Description |
|---|---|---|
| `PORT` | `6270` | HTTP listen port |
| `DB_PATH` | `gitoverviewer.db` | SQLite database path |
| `GITHUB_CLIENT_ID` | — | GitHub OAuth App client ID |
| `GITHUB_CLIENT_SECRET` | — | GitHub OAuth App client secret |
| `GITHUB_REDIRECT_URL` | `http://localhost:6270/auth/github/callback` | OAuth callback URL |
| `GITHUB_WEBHOOK_SECRET` | — | HMAC-SHA256 secret for webhook verification |

## Docker

```bash
docker compose up --build
```

The Docker Compose setup mounts volumes for persistent database and data storage. The app is available on port 6270.

## Development

### Available tasks

| Task | Description |
|---|---|
| `task run` | Run the application |
| `task dev` | Run with hot-reload (Air) |
| `task test` | Run Go unit tests |
| `task test:e2e` | Run Playwright E2E tests |
| `task build` | Compile TypeScript and build Go binary |
| `task fmt` | Format Go code |
| `task generate` | Regenerate Ent ORM code from schema |
| `task tidy` | Tidy Go modules |
| `task docker:build` | Build Docker image |

### Running tests

```bash
# Unit tests
task test

# E2E tests (requires built binary)
task build
task test:e2e
```

## Project Structure

```
.
├── main.go                  # Application entry point
├── internal/
│   ├── github/              # GitHub API client
│   ├── handlers/            # HTTP handlers (auth, dashboard, settings, webhook)
│   ├── middleware/           # Session management middleware
│   ├── sync/                # Data synchronization engine
│   └── ws/                  # WebSocket hub
├── ent/
│   ├── schema/              # Ent ORM schemas (User, Repository)
│   └── ...                  # Auto-generated Ent code
├── static/                  # Static files served to browser
│   ├── index.html           # Main HTML template (Go templates)
│   ├── style.css            # Stylesheet
│   └── favicon.ico          # Favicon
├── ts/                      # TypeScript source files
├── e2e/                     # Playwright E2E tests
├── Taskfile.yml             # Task runner tasks
├── Dockerfile               # Multi-stage Docker build
├── docker-compose.yml       # Docker Compose config
└── .releaserc.yaml          # Semantic release configuration
```

## Routes

| Route | Method | Description |
|---|---|---|
| `/` | GET | Home / dashboard |
| `/auth/github` | GET | GitHub OAuth login |
| `/auth/github/callback` | GET | OAuth callback |
| `/auth/logout` | POST | Logout |
| `/dashboard` | GET | Dashboard partial |
| `/repos` | GET | Repository list (HTMX partial) |
| `/repos/{id}/sync` | POST | Sync single repo |
| `/repos/import-all` | POST | Import all GitHub repos |
| `/repos/{id}` | DELETE | Remove tracked repo |
| `/settings` | GET | Settings page |
| `/settings/interval` | POST | Update sync interval |
| `/settings/repos/available` | GET | List available repos |
| `/settings/repos/select` | POST | Import selected repos |
| `/webhook/github` | POST | GitHub push webhook |
| `/ws` | GET | WebSocket endpoint |

## License

MIT
