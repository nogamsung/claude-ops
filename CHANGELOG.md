# Changelog

## [0.3.0](https://github.com/nogamsung/claude-ops/compare/v0.2.1...v0.3.0) (2026-04-24)


### Features

* initial MVP — scheduled-dev-agent + CI/CD ([ac64ab7](https://github.com/nogamsung/claude-ops/commit/ac64ab79727a98fb7441821744e631381313a98f))
* initial MVP — scheduled-dev-agent + CI/CD ([4ed3e4d](https://github.com/nogamsung/claude-ops/commit/4ed3e4dacf604364fe2056d0493322af7948f24a))
* **scheduler:** daily/weekly task budget gate + rate-limit auto throttle ([178f11a](https://github.com/nogamsung/claude-ops/commit/178f11a793571fb7b740d99927e58e4cbecd47aa))
* **scheduler:** daily/weekly task budget gate + rate-limit auto throttle ([32a6cab](https://github.com/nogamsung/claude-ops/commit/32a6cab8396bce4d5d38e062317d7f4d84c4a8cf))


### Bug Fixes

* **docker:** bump builder image to golang:1.25-alpine ([5e22a5d](https://github.com/nogamsung/claude-ops/commit/5e22a5dbca2da03c33ee1933cc67f8c2f24a76c3))
* **docker:** bump builder image to golang:1.25-alpine ([26885bf](https://github.com/nogamsung/claude-ops/commit/26885bf70fbf38b175d67705c5190138888b746e))


### Code Refactoring

* rename project from scheduled-dev-agent to Claude Ops ([979455f](https://github.com/nogamsung/claude-ops/commit/979455f7901f45f0d659e8218a230e25ff1608c6))
* rename project to Claude Ops ([d3ae29e](https://github.com/nogamsung/claude-ops/commit/d3ae29e446325f667d07362a36834e5dab732f8c))


### Continuous Integration

* **lint:** upgrade golangci-lint to v2.5.0 (supports Go 1.25+) ([c31ea49](https://github.com/nogamsung/claude-ops/commit/c31ea495cd22e2aef3b16c69d79f401ff070b51f))
* **release:** add release-please for automated versioning ([c10e5c8](https://github.com/nogamsung/claude-ops/commit/c10e5c824f1b0be89ed34e10466cb6e5d9b70e84))
* **release:** add release-please for automated versioning ([e75cbc5](https://github.com/nogamsung/claude-ops/commit/e75cbc591e66a8916fa647b595bedd1d62b84d0c))
* **release:** add workflow_dispatch to re-publish missed release tags ([#9](https://github.com/nogamsung/claude-ops/issues/9)) ([1620cab](https://github.com/nogamsung/claude-ops/commit/1620cab8c5536df5202d1cd7db869b265c1b5fbe))
* **release:** chain docker publish inside release-please workflow ([#7](https://github.com/nogamsung/claude-ops/issues/7)) ([9dca6ed](https://github.com/nogamsung/claude-ops/commit/9dca6ed9673ed603202269d8d480a31e89e15e19))
* **release:** emit only vX.Y.Z + latest for release images ([#16](https://github.com/nogamsung/claude-ops/issues/16)) ([b99b58f](https://github.com/nogamsung/claude-ops/commit/b99b58f81348bd0e47b17af4c50999466d2447e5))


### Miscellaneous Chores

* **deploy:** add Docker deploy.sh for home-server rollout ([#15](https://github.com/nogamsung/claude-ops/issues/15)) ([3632453](https://github.com/nogamsung/claude-ops/commit/36324536ebdb08413eed14032e13f0ab07f51ff3))
* **harness:** scaffold Go single-stack Claude Code harness ([#8](https://github.com/nogamsung/claude-ops/issues/8)) ([5ed6733](https://github.com/nogamsung/claude-ops/commit/5ed673326eaf3a502e4a0dfeeb3f1dd95605e96f))
* **main:** release 0.2.0 ([ca8c856](https://github.com/nogamsung/claude-ops/commit/ca8c856329a0a0bd37e0f27ab5e1afbdc579cc35))
* **main:** release 0.2.0 ([0c1fccf](https://github.com/nogamsung/claude-ops/commit/0c1fccf7277b1e6801d8b9e0423f471f8438340b))
* **main:** release 0.2.1 ([#17](https://github.com/nogamsung/claude-ops/issues/17)) ([2df4bb6](https://github.com/nogamsung/claude-ops/commit/2df4bb6028c1edad7d4ec0a42fc81fb7ac9ca7f0))

## [0.2.1](https://github.com/nogamsung/claude-ops/compare/v0.2.0...v0.2.1) (2026-04-24)


### Continuous Integration

* **release:** chain docker publish inside release-please workflow ([#7](https://github.com/nogamsung/claude-ops/pull/7)) ([9dca6ed](https://github.com/nogamsung/claude-ops/commit/9dca6ed9673ed603202269d8d480a31e89e15e19))
* **release:** add workflow_dispatch to re-publish missed release tags ([#9](https://github.com/nogamsung/claude-ops/pull/9)) ([1620cab](https://github.com/nogamsung/claude-ops/commit/1620cab8c5536df5202d1cd7db869b265c1b5fbe))
* **release:** emit only vX.Y.Z + latest for release images ([#16](https://github.com/nogamsung/claude-ops/pull/16)) ([b99b58f](https://github.com/nogamsung/claude-ops/commit/b99b58f))


### Miscellaneous Chores

* **harness:** scaffold Go single-stack Claude Code harness ([#8](https://github.com/nogamsung/claude-ops/pull/8)) ([5ed6733](https://github.com/nogamsung/claude-ops/commit/5ed6733))
* **deploy:** add Docker deploy.sh for home-server rollout ([#15](https://github.com/nogamsung/claude-ops/pull/15)) ([3632453](https://github.com/nogamsung/claude-ops/commit/3632453))


## [0.2.0](https://github.com/nogamsung/claude-ops/compare/v0.1.0...v0.2.0) (2026-04-19)


### Features

* **scheduler:** daily/weekly task budget gate + rate-limit auto throttle ([178f11a](https://github.com/nogamsung/claude-ops/commit/178f11a793571fb7b740d99927e58e4cbecd47aa))
* **scheduler:** daily/weekly task budget gate + rate-limit auto throttle ([32a6cab](https://github.com/nogamsung/claude-ops/commit/32a6cab8396bce4d5d38e062317d7f4d84c4a8cf))
