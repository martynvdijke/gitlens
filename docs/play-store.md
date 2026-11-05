# Google Play submission notes

This document collects the facts needed to publish the GitLens Android app and
to answer the Play Console declarations accurately. It describes the real
GitLens data model, not a generic template.

## App summary

GitLens is a self-hosted dashboard for GitHub and Forgejo repositories. The web
application signs in with GitHub, Forgejo or a configured OIDC provider, reads
the repositories a user chooses to track, and computes metrics such as commit
type counts, lead time and workflow success rates.

The Android app is a client for a GitLens server that the user configures. It
does not ship with a bundled backend or a fixed account: the user enters the
server URL, then signs in through that server's GitHub, Forgejo or OIDC flow.
The canonical instance is <https://gitlens.vandijke.xyz>, but self-hosted
instances are supported.

- Application ID: `xyz.vandijke.gitlens`
- Android version name / code: read `version:` from `mobile/pubspec.yaml`
  (currently `1.42.2+1`; the Play version code is supplied at build time).
- Only Android permission: `android.permission.INTERNET`.
- `android:allowBackup="false"` is set so session and account material is not
  included in Android backups.

## Required Play Console URLs

Replace the host if you publish from a different instance.

| Field | URL |
| --- | --- |
| Privacy policy | `https://gitlens.vandijke.xyz/privacy` |
| Account deletion | `https://gitlens.vandijke.xyz/account/delete` |
| Terms of use | `https://gitlens.vandijke.xyz/terms` |

These routes are public and render without a session, which is required so
reviewers can read them before signing in. The deletion page is the public web
deletion route Google Play asks for.

## Sign-in and test account

Reviewers have two options.

1. **Shared demo account.** On the web, choose **Try the demo** (the button is
   shown when the demo account is enabled); the route is `/auth/demo`. In the
   Android app, choose **Try the demo** on the onboarding screen.
   - Login: `demo`
   - Public token: `glt_demo_public_preview_2026`
   - This token is intentionally public and only ever unlocks generated sample
     data. It is stored server-side as a SHA-256 hash.
   - Server configuration: set `DEMO_USER_ENABLED=true` and a correct `APP_URL`.
     When `DEMO_USER_ENABLED` is unset the demo account is enabled only when
     `APP_URL` points at the canonical `gitlens.vandijke.xyz` host.
   - Deleting the demo account resets it to the sample state, so the next
     reviewer still has a working demo.
2. **A real account.** Reviewers may sign in with their own GitHub, Forgejo or
   OIDC account through the normal flow. This is the best way to exercise
   account deletion end to end, because the demo account self-heals instead of
   staying deleted.

Do not put a private credential in the review notes. The demo token above is
public by design and contains no personal data.

## Data safety

Declare the following in the Play Console Data safety form. Every category is
collected for app functionality or security, is transmitted over HTTPS, is not
sold, and is not shared for advertising.

| Data type | What GitLens actually handles | Collected? | Purpose |
| --- | --- | --- | --- |
| Account identifiers | GitHub/Forgejo numeric user ID, login, display name, avatar URL | Yes | Account management, sign-in |
| Email address | Only when the configured OIDC provider returns an email claim; GitHub/Forgejo flows may not provide one | Yes, conditionally | Account management |
| Authentication tokens | GitHub and Forgejo OAuth access tokens; stored so GitLens can read repositories on the user's behalf; never displayed to other users | Yes | App functionality |
| Repository content | Tracked repository metadata, commit messages, pull requests, releases, workflow status | Yes | App functionality |
| Derived data | Commit-type counts, lead time, workflow success/failure counts, metric snapshots | Yes | App functionality |
| App-created credentials | GitLens API tokens, stored only as SHA-256 hashes | Yes | App functionality, security |
| Session and diagnostics | Session cookie and server-side session row, request IP address, server logs used for rate limiting and failure diagnosis | Yes | Security, diagnostics |
| Analytics configuration | Optional Umami URL and site ID entered by the user; cookieless page views | Yes, optional | App functionality (the user's own analytics) |
| Deploy notifications | Optional Gotify/Telegram release and workflow metadata configured by the operator | Yes, optional | App functionality |

Additional declarations:

- Data is **encrypted in transit** (TLS). Production deployments must use HTTPS.
- Data is **not sold** and **not shared** with third parties for advertising or
  cross-site tracking.
- Data is **not used for advertising or marketing**.
- Users can delete their data **in the app** (Settings → Delete account) and on
  the **web** (`/account/delete`), and can export it at `/api/v1/me/export`.
- Data is retained only while the account exists, plus short-lived sessions,
  rotated logs and rotating backups.

Not collected, and should not be declared:

- Precise or approximate location
- Photos, videos, audio, or files on the device
- Contacts, calendar, or SMS/email messages
- Microphone, camera, or health data
- Financial or payment information
- Installed-app inventory or device advertising ID

## Permissions

| Permission | Why it is declared |
| --- | --- |
| `android.permission.INTERNET` | Required to reach the GitLens server the user configures, and from there GitHub, Forgejo or the OIDC provider. |

Notes for the review:

- `android:usesCleartextTraffic="true"` exists so a self-hosted development
  server can be reached over plain HTTP. Production instances, including the
  canonical one, use HTTPS. This is not a request for any runtime permission.
- `android:allowBackup="false"` is set so account tokens and session material
  are not copied into Android cloud backups.
- The app requests no runtime permissions, no location, no storage and no
  notification permission.

## Content rating and target audience

- Category: **Tools / Productivity** (utility app for developers).
- GitLens itself does not publish user-generated content. It displays
  repository data that the user is already authorised to read from GitHub or
  Forgejo.
- No ads, no in-app purchases, no gambling, no violence, no sexual content, no
  profanity, no drugs or alcohol references.
- Intended audience: **developers aged 16 and over**. The web legal pages state
  that the instance is not intended for anyone under 16. Complete the content
  rating questionnaire consistently with this: utility/productivity, no
  user-generated content published by the app, no ads, target audience 16+.

## Account deletion requirements

Google Play requires both an in-app path and a public web page.

- **In-app:** Settings (account icon) → **Delete account**, then type the
  username to confirm. This calls the same endpoint as the web UI.
- **Web:** <https://gitlens.vandijke.xyz/account/delete>.
- Confirmation is **immediate**; there is no waiting or retention period.
- Deleting the **shared demo account** resets it to the sample state so the
  public demo stays available. It contains only generated sample data.
- The **last administrator cannot be deleted**, because that would leave nobody
  able to manage the instance. Promote or create another administrator first.
- After deletion, the user's provider-side repositories are untouched; deleting
  GitLens does not delete anything on GitHub or Forgejo.

The privacy policy explains the GDPR Article 17 erasure right, and the deletion
page links to the export endpoint for users who want a copy first.

## Closed testing and production access

Google Play requires new **personal** developer accounts to run a closed test
with at least **12 testers for 14 continuous days** before applying for
production access. Organisation accounts may have different requirements,
but plan for the personal-account rule unless the account is already verified.

1. Create the app in Play Console with package `xyz.vandijke.gitlens`.
2. Upload a signed AAB to the **internal** or **closed** track.
3. Add at least 12 testers (email list or Google Group), keep the test running
   for 14 continuous days, and collect feedback.
4. Apply for production access and complete the store listing, data safety,
   content rating and account-deletion declarations.
5. Promote the release to production once approved.

If CI credentials are absent, build and upload manually:

```bash
cd mobile
flutter pub get
flutter build appbundle --release \
  --build-name "<version from pubspec>" \
  --build-number "<monotonically increasing Play version code>"
# Upload mobile/build/app/outputs/bundle/release/app-release.aab in Play Console.
```

## CI publishing

The existing workflow is `.github/workflows/play-store.yaml`, called from the
release workflow after semantic-release publishes a version. It:

- checks for `ANDROID_KEYSTORE_BASE64` and `GOOGLE_PLAY_SERVICE_ACCOUNT_JSON`,
  and skips publishing with a warning when either is missing;
- decodes the upload keystore from `ANDROID_KEYSTORE_BASE64` and writes
  `mobile/android/key.properties` using `ANDROID_KEYSTORE_PASSWORD`,
  `ANDROID_KEY_ALIAS` and `ANDROID_KEY_PASSWORD`;
- builds a signed AAB with `flutter build appbundle --release`;
- uploads it with `r0adkll/upload-google-play` using
  `GOOGLE_PLAY_SERVICE_ACCOUNT_JSON`;
- uses `vars.ANDROID_PACKAGE_NAME` (default `xyz.vandijke.gitlens`),
  `vars.GOOGLE_PLAY_TRACK` (default `internal`) and
  `vars.GOOGLE_PLAY_RELEASE_STATUS` (default `completed`).

The Play Console service account needs the **Release to testing tracks**
permission (and production release permission if the track is promoted to
production). Grant it access to this app only.

## Pre-submission checklist

- [ ] Set the `LEGAL_*` environment variables on the deployed instance:
  `LEGAL_CONTROLLER_NAME`, `LEGAL_CONTACT_EMAIL`, `LEGAL_CONTACT_ADDRESS`,
  `LEGAL_HOSTING_REGION`, `LEGAL_DPO_EMAIL`, `LEGAL_EU_REPRESENTATIVE`,
  `LEGAL_SUPERVISORY_AUTHORITY`.
- [ ] Deploy and verify `https://<host>/privacy` renders the EU section, the
  controller, the EU representative, the DPO contact and the supervisory
  authority.
- [ ] Verify `https://<host>/account/delete` shows the Google Play, GDPR and demo
  paragraphs and still explains the last-admin refusal.
- [ ] Verify `https://<host>/api/v1/auth/config` reports the expected sign-in
  providers and demo availability for the app.
- [ ] Verify the demo login (`demo` / `glt_demo_public_preview_2026`) works and
  that deleting the demo account resets it.
- [ ] Exercise deletion once with a real test account.
- [ ] Confirm HTTPS on the public host and that the app connects to it.
- [ ] Build a signed AAB (CI or manual) and confirm the version code increases.
- [ ] Complete Data safety, Content rating, Target audience and Account deletion
  declarations in Play Console.
- [ ] Run the closed test for 14 continuous days with at least 12 testers before
  applying for production access.
