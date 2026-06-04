## [1.7.3](https://github.com/martynvdijke/gitlens/compare/v1.7.2...v1.7.3) (2026-06-04)

## [1.7.2](https://github.com/martynvdijke/gitlens/compare/v1.7.1...v1.7.2) (2026-06-03)

## [1.7.1](https://github.com/martynvdijke/gitlens/compare/v1.7.0...v1.7.1) (2026-06-01)


### Bug Fixes

* cumulative commit counts, drop unique github_id, fix SVG escaping ([7f1547e](https://github.com/martynvdijke/gitlens/commit/7f1547e8d6d8c87dbb16ced3e6d6f78072c4ca35))

# [1.7.0](https://github.com/martynvdijke/gitlens/compare/v1.6.2...v1.7.0) (2026-05-31)


### Features

* auto-show setup on login, session timezone fix, rate-limited commits ([b12d1dd](https://github.com/martynvdijke/gitlens/commit/b12d1dd8d4e2720d0e6c69e4eccad17b43216f05))

## [1.6.2](https://github.com/martynvdijke/gitlens/compare/v1.6.1...v1.6.2) (2026-05-30)


### Bug Fixes

* SVG escaping, session persistence, WS scope, and pagination ([19081a7](https://github.com/martynvdijke/gitlens/commit/19081a7bfcb0270022544d7e135661e24c74511f))

## [1.6.1](https://github.com/martynvdijke/gitlens/compare/v1.6.0...v1.6.1) (2026-05-29)


### Bug Fixes

* prevent blank page on refresh with HTMX redirect middleware, improve mobile layout ([c6db9fc](https://github.com/martynvdijke/gitlens/commit/c6db9fca6e6e68331d88735389c373c8527ac3da))

# [1.6.0](https://github.com/martynvdijke/gitlens/compare/v1.5.4...v1.6.0) (2026-05-28)


### Bug Fixes

* update Playwright tests for Bootstrap Darkly UI migration ([c7e1a53](https://github.com/martynvdijke/gitlens/commit/c7e1a53309e8d1927d862d15221da4e7bac68d35))


### Features

* add Bootstrap Darkly CSS framework with Material-inspired UI ([9da26ff](https://github.com/martynvdijke/gitlens/commit/9da26ff624b7d8fc87ff7bbee5ccab8c284ff35d))
* tab-navigation-redesign with cross-repo PR queue and metrics tab ([9e9113a](https://github.com/martynvdijke/gitlens/commit/9e9113af59282ceb561b579cc4153e14e20bedcb))

## [1.5.4](https://github.com/martynvdijke/gitlens/compare/v1.5.3...v1.5.4) (2026-05-26)

## [1.5.3](https://github.com/martynvdijke/gitlens/compare/v1.5.2...v1.5.3) (2026-05-25)


### Bug Fixes

* **deps:** update all non-major dependencies ([#1](https://github.com/martynvdijke/gitlens/issues/1)) ([971c0d1](https://github.com/martynvdijke/gitlens/commit/971c0d159a93631580decd3b6924b599aeb3cf12))

## [1.5.2](https://github.com/martynvdijke/gitlens/compare/v1.5.1...v1.5.2) (2026-05-25)


### Bug Fixes

* invalid timezone UTC+1, use Europe/Amsterdam instead ([cc23ee8](https://github.com/martynvdijke/gitlens/commit/cc23ee87d8afefd6932b4ae294cdb4709e4a6863))
* remove stalePr and stalePrAge from renovate.json - removed in Renovate v37 ([ed0c000](https://github.com/martynvdijke/gitlens/commit/ed0c000c388cea345a9363a15da39455ecbd0b38))

## [1.5.1](https://github.com/martynvdijke/gitlens/compare/v1.5.0...v1.5.1) (2026-05-25)


### Bug Fixes

* version footer, import error logging, add import/merge tests ([9c6d6b9](https://github.com/martynvdijke/gitlens/commit/9c6d6b97b108bbeeca306ffb022c0641a9c190ab))

# [1.5.0](https://github.com/martynvdijke/gitlens/compare/v1.4.0...v1.5.0) (2026-05-24)


### Features

* persist sessions in database so auth survives restarts ([c21eb4d](https://github.com/martynvdijke/gitlens/commit/c21eb4d96f3eb396a074d1dd9059e25f2b45cc12))

# [1.4.0](https://github.com/martynvdijke/gitlens/compare/v1.3.0...v1.4.0) (2026-05-24)


### Features

* show app version in footer via ldflags ([d6d6ef4](https://github.com/martynvdijke/gitlens/commit/d6d6ef499876b1465da24b63de304edd00b476ce))

# [1.3.0](https://github.com/martynvdijke/gitlens/compare/v1.2.0...v1.3.0) (2026-05-24)


### Bug Fixes

* rebuild binary before e2e tests and fix feed redirect tests ([13a9356](https://github.com/martynvdijke/gitlens/commit/13a9356fe26e594dbb17fe41ded47f636098cfd4))


### Features

* add activity feed with timeline, filters, tests ([892c447](https://github.com/martynvdijke/gitlens/commit/892c447d5ba212b654de173a7f9f5fb14f00da4b))

# [1.2.0](https://github.com/martynvdijke/gitlens/compare/v1.1.1...v1.2.0) (2026-05-23)


### Features

* add webhook, settings handler tests and expand github client coverage ([248cb6f](https://github.com/martynvdijke/gitlens/commit/248cb6f5b86818cda5a60512871c20a6fa73a9a2))

## [1.1.1](https://github.com/martynvdijke/gitlens/compare/v1.1.0...v1.1.1) (2026-05-23)


### Bug Fixes

* multi-repo import only syncs new repos, add release workflow status and PR tests ([1f6efa5](https://github.com/martynvdijke/gitlens/commit/1f6efa51a3cbbfedab23cf8c1a6604e6c87d0dc5))

# [1.1.0](https://github.com/martynvdijke/gitlens/compare/v1.0.1...v1.1.0) (2026-05-21)


### Features

* add DORA metrics charts with SVG rendering and tests ([784b1b4](https://github.com/martynvdijke/gitlens/commit/784b1b4a86ee108759baf81e313860daf475d964))
* add GitHub App integration for auto-import and webhook setup ([c2def3b](https://github.com/martynvdijke/gitlens/commit/c2def3b38ae14e2915558bfdc76f55dfa443e404))
* add repo search/filter with live HTMX search bar and tests ([9d5f6c9](https://github.com/martynvdijke/gitlens/commit/9d5f6c97fe7dd823747ba05521585f625da7235b))
* add status badge API endpoint for README embedding ([09fb249](https://github.com/martynvdijke/gitlens/commit/09fb249e10e2307a9e2dd32782f0db19d40e3f63))
* enhance landing page, add loading skeletons, improve mobile UX ([3f5145d](https://github.com/martynvdijke/gitlens/commit/3f5145d92e6def3821fc90969392689d689c3058))

## [1.0.1](https://github.com/martynvdijke/gitlens/compare/v1.0.0...v1.0.1) (2026-05-20)


### Bug Fixes

* ensure Gotify notification always fires on release workflow ([5dfa529](https://github.com/martynvdijke/gitlens/commit/5dfa529a7a86209ff5578715c4127cb850734184))

# 1.0.0 (2026-05-20)


### Bug Fixes

* add callback ([501ebd0](https://github.com/martynvdijke/gitlens/commit/501ebd04bd5e1c143b17dcadfe079eafa24c215b))
* add lockfile ([0249d4a](https://github.com/martynvdijke/gitlens/commit/0249d4ac323b402364cdd867d615074567d1622e))
* **ci:** trigger ci for release ([6ccf6bf](https://github.com/martynvdijke/gitlens/commit/6ccf6bf79b031cf42afba1f03d630727c1dcc1e7))
* **ci:** trigger ci for release ([adbd315](https://github.com/martynvdijke/gitlens/commit/adbd315e4e93df8dc52dffcd87b886cda2c07c64))
* fix ([d31cdea](https://github.com/martynvdijke/gitlens/commit/d31cdea5ea9af5f75d200c82b95271162262ad18))
* rebuild gitlens binary and update .gitignore ([b4d2287](https://github.com/martynvdijke/gitlens/commit/b4d2287cf3824250337e3e9c2bb4dde307ea5811))
* renovate ([546335a](https://github.com/martynvdijke/gitlens/commit/546335ab1d353e3fa5eaf1e112f5be7223f82527))
* **tests:** accept 404 for favicon, check server redirect for login ([96c2869](https://github.com/martynvdijke/gitlens/commit/96c28695e4d97a48ec29b7b59c90fc7ded8ec1c1))
* use _journal_mode=wal instead of invalid mode=wal in sqlite DSN ([bf31cc1](https://github.com/martynvdijke/gitlens/commit/bf31cc1f3d183b3c5e5b5e866684bd09b69e503e))


### Features

* first release ([d99717e](https://github.com/martynvdijke/gitlens/commit/d99717e0e0d0c091d1b6a1ec84c822ce197d1645))

# 1.0.0 (2026-05-20)


### Bug Fixes

* add callback ([501ebd0](https://github.com/martynvdijke/gitlens/commit/501ebd04bd5e1c143b17dcadfe079eafa24c215b))
* add lockfile ([0249d4a](https://github.com/martynvdijke/gitlens/commit/0249d4ac323b402364cdd867d615074567d1622e))
* **ci:** trigger ci for release ([adbd315](https://github.com/martynvdijke/gitlens/commit/adbd315e4e93df8dc52dffcd87b886cda2c07c64))
* fix ([d31cdea](https://github.com/martynvdijke/gitlens/commit/d31cdea5ea9af5f75d200c82b95271162262ad18))
* rebuild gitlens binary and update .gitignore ([b4d2287](https://github.com/martynvdijke/gitlens/commit/b4d2287cf3824250337e3e9c2bb4dde307ea5811))
* renovate ([546335a](https://github.com/martynvdijke/gitlens/commit/546335ab1d353e3fa5eaf1e112f5be7223f82527))
* **tests:** accept 404 for favicon, check server redirect for login ([96c2869](https://github.com/martynvdijke/gitlens/commit/96c28695e4d97a48ec29b7b59c90fc7ded8ec1c1))
* use _journal_mode=wal instead of invalid mode=wal in sqlite DSN ([bf31cc1](https://github.com/martynvdijke/gitlens/commit/bf31cc1f3d183b3c5e5b5e866684bd09b69e503e))


### Features

* first release ([d99717e](https://github.com/martynvdijke/gitlens/commit/d99717e0e0d0c091d1b6a1ec84c822ce197d1645))
