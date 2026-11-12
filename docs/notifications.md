# Push notifications

GitLens can notify you, in real time, when a new activity item appears in the
feed (a new release, a merged pull request, or a failed workflow). Two delivery
channels are supported:

| Channel | How it reaches you |
|---|---|
| **Web** | Browser Web Notifications, pushed over a dedicated WebSocket (`/ws/notifications`) |
| **Android** | Firebase Cloud Messaging (FCM) push to the GitLens Android app |

Both channels share the same per-user preferences and the same server-side
filtering, so a user who disables releases never hears about them on either
channel.

## How it works

```
sync / webhook  →  Event row created
                      │
                      ▼
             internal/notify.Notifier
        (global → event type → per-user gates)
                 │                    │
                 ▼                    ▼
      WebSocket broadcast        FCM HTTP v1 send
      {"type":"notification"}    to DeviceToken rows
                 │                    │
                 ▼                    ▼
        Browser Notification     Android notification
```

1. `internal/sync` records an `Event` (release, `pr_merge`, or
   `workflow_failure`) exactly as it did before.
2. After the row is saved, the syncer hands the event to the notifier.
3. The notifier checks, in order: the global admin switch, the global per-type
   switch, and then each user's own preferences and event-type switches.
4. Matching users get a JSON message on the notification hub; users with an
   Android device token registered also get an FCM push.

Delivery is best-effort and never fails a sync: notify errors are logged only.

## Admin setup

Notification settings live in **Settings → Administration → Notifications**
(admins only). Secrets are encrypted at rest with the same key used for other
app secrets (`SETTINGS_ENCRYPTION_KEY`, falling back to `SESSION_SECRET`); a
saved secret is never echoed back to the browser.

### 1. Create a Firebase project and Android app

1. In the [Firebase console](https://console.firebase.google.com/), create a
   project (or reuse one) and add an **Android** app with package name
   `xyz.vandijke.gitlens` (the app's applicationId).
2. From **Project settings → General → Your apps**, note the values the app
   needs at runtime:
   - **Project ID** (`fcm_project_id`)
   - **API key** (`fcm_api_key`)
   - **App ID** (`fcm_app_id`, looks like `1:1234567890:android:abc123`)
   - **Sender ID** (`fcm_sender_id`, the numeric project number)
   - **Storage bucket** (`fcm_storage_bucket`, optional)
3. Enable **Cloud Messaging** for the project. The legacy server key is not
   used: GitLens sends through the HTTP v1 API.

### 2. Create a service account for the server

1. **Project settings → Service accounts → Generate new private key** and
   download the JSON file.
2. Paste that JSON into the **Service account JSON** field and save. The server
   signs a JWT with the key, exchanges it for an OAuth2 token, and caches the
   token until shortly before expiry.

Grant the service account the Firebase Cloud Messaging sender role (the
default Firebase Admin SDK service account already has it).

### 3. Turn notifications on

The **Notifications** admin card has:

- a master switch (`notifications_enabled`),
- per-type switches for **Releases**, **Merged pull requests**, and
  **Workflow failures**,
- a **Send test notification** button that pushes to the requesting admin's own
  devices, so setup can be verified end to end,
- the FCM client configuration above.

If the master switch is off, nothing is broadcast or pushed regardless of user
preferences. If FCM is not configured, web notifications still work and Android
registration silently does nothing.

## User settings

Each user gets a **Notifications** card in **Settings** with:

- a master switch (`notifications_enabled`),
- **Browser notifications** (`notify_web`),
- **Android push** (`notify_android`),
- per-event switches for releases, merged PRs, and workflow failures.

Per-event switches are ignored while the master switch is off. Defaults are all
on, so an existing account starts receiving notifications as soon as the admin
enables them.

### Enabling browser notifications

Browsers only grant the Notification permission in response to a user gesture.
The settings card includes an **Enable browser notifications** button; it asks
for permission and shows the result. If permission was previously denied, the
browser will not prompt again — the user must reset it in the site settings.

The web client connects to `/ws/notifications`, reconnects with exponential
backoff, and refreshes the feed (HTMX) when a notification arrives so the page
stays consistent with the toast.

## API

All endpoints except the public config endpoint require the normal
session cookie or `Authorization: Bearer glt_…` token.

| Route | Method | Description |
|---|---|---|
| `/api/v1/notifications/config` | GET | Public Firebase client config (no secrets). Returns `enabled: false` when unconfigured |
| `/api/v1/notifications/prefs` | GET | Current user's notification preferences |
| `/api/v1/notifications/prefs` | POST | Update preferences (JSON body) |
| `/api/v1/notifications/device` | POST | Register a device token (`{token, platform}`) |
| `/api/v1/notifications/device` | DELETE | Unregister a device token (`{token}`) |
| `/ws/notifications` | GET | WebSocket for browser notifications |
| `/settings/notifications` | POST | HTMX form: save browser preferences |
| `/admin/notifications` | POST | Save global + FCM settings |
| `/admin/notifications/test` | POST | Send a test notification to the caller |

### Payloads

Preferences:

```json
{
  "notifications_enabled": true,
  "notify_web": true,
  "notify_android": true,
  "notify_release": true,
  "notify_pr_merge": true,
  "notify_workflow_failure": true
}
```

Client config:

```json
{
  "enabled": true,
  "project_id": "my-project",
  "api_key": "AIza…",
  "app_id": "1:123:android:abc",
  "sender_id": "123",
  "storage_bucket": "my-project.appspot.com"
}
```

WebSocket message:

```json
{
  "type": "notification",
  "event": {
    "id": 42,
    "type": "release",
    "title": "acme/app v1.2.3",
    "url": "https://github.com/acme/app/releases/tag/v1.2.3",
    "repo": "acme/app",
    "timestamp": "2026-01-02T15:04:05Z"
  }
}
```

Device registration is idempotent per token: re-registering refreshes
`last_seen_at`, and a token that FCM rejects as invalid is deleted.

## Android app

The app has no `google-services.json` and applies no Google Services Gradle
plugin. On sign-in it fetches `/api/v1/notifications/config`, builds
`FirebaseOptions` from it, initializes Firebase at runtime, requests the
`POST_NOTIFICATIONS` permission (Android 13+), and registers its FCM token.
Foreground messages show a snackbar; background messages use the system
notification tray. Signing out unregisters the token so the next user on the
device does not inherit notifications.

## Security notes

- The service account JSON and any other secret are stored encrypted; the
  admin form shows "saved — leave blank to keep" placeholders instead of
  values.
- `/api/v1/notifications/config` exposes only public Firebase client
  identifiers, which are not secret by design. It returns no project secrets
  and no service account material.
- The notification WebSocket authenticates exactly like `/ws` (session cookie
  or bearer token) and only ever carries events the user is allowed to see
  (their tracked repositories).
- Device tokens are scoped to the user who registered them; unregistering one
  user's token cannot affect another user's.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| No browser toasts | Notification permission not granted, or `notify_web` / master switch off |
| Web toasts work, Android silent | FCM config incomplete, or `notify_android` off |
| `/api/v1/notifications/config` returns `enabled: false` | Project ID, API key, app ID or sender ID missing |
| FCM returns 401/403 | Service account JSON invalid, or the account lacks the Cloud Messaging role |
| FCM reports an invalid token | App was reinstalled or the token was rotated; the row is deleted and the app re-registers on next start |
| Notifications stop after a deploy | The browser WebSocket reconnects automatically; a hard-refreshed page re-registers |
