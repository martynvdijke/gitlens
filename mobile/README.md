# GitLens for Android

A Flutter client for your self-hosted [GitLens](https://github.com/martynvdijke/gitlens) server.

There is **no default server and no hosted service**. The first screen asks for the address of your own
GitLens instance, and only the address you enter is ever contacted.

## Signing in

Two options are offered on the onboarding screen:

- **OIDC / SSO** — when the server reports OIDC as enabled, the app opens the server's OIDC login in an
  in-app WebView. The resulting HttpOnly `gitlens_session_oidc` cookie is read through a native Android
  cookie bridge, stored with `flutter_secure_storage`, and sent only to your server.
- **API token** — paste a personal token created under **Settings → API tokens** in GitLens. Tokens are
  sent as `Authorization: Bearer <token>`.

## Features

- Live repo wall: dashboard overview, repository cards, sync and Renovate rebase actions, WebSocket refresh.
- PR triage: merge, close, rerun checks, batch actions, and merge-passing.
- Activity feed with time range and event type filters.
- DORA metrics and year overview with charts.

## Development

```bash
cd mobile
flutter pub get
flutter test
flutter analyze
flutter run
```

Requires Flutter 3.47.7 or newer and Java 17 for Android builds.

## Release APK

```bash
flutter build apk --release
```

The signed build reads `android/key.properties` when present. Without it, Android falls back to debug
signing so local builds still work. The `Flutter Android` workflow runs analyze, tests, and the APK build
on pull requests; releases attach `gitlens-<version>-android.apk` to the GitHub release.

Optional release secrets: `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`,
`ANDROID_KEY_ALIAS`, and `ANDROID_KEY_PASSWORD`.

## Branding

The launcher icon and in-app mark reuse the server's `static/logo.svg` favicon: the green ring, blue
crosshair, and centre dot on the GitLens dark `#161b22` background. Android also ships a themed
monochrome icon for Android 13+.
