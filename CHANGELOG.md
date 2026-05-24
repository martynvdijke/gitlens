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
