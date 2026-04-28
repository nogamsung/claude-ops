# Changelog

## [0.3.1](https://github.com/nogamsung/claude-ops/compare/v0.3.0...v0.3.1) (2026-04-28)


### Miscellaneous Chores

* **deploy:** switch host path prefix to nogamsung ([#28](https://github.com/nogamsung/claude-ops/issues/28)) ([411270f](https://github.com/nogamsung/claude-ops/commit/411270fa1de73b02afcb0d85b26810444b6539f4))

## [0.3.0](https://github.com/nogamsung/claude-ops/compare/v0.2.1...v0.3.0) (2026-04-28)


### Features

* **cli:** claude-opsctl CLI (closes [#14](https://github.com/nogamsung/claude-ops/issues/14)) ([#21](https://github.com/nogamsung/claude-ops/issues/21)) ([4c8638f](https://github.com/nogamsung/claude-ops/commit/4c8638f1fecbef57ad78964ee32b9e7e05a5238a))
* **gc:** worktree GC + orphan recovery (closes [#12](https://github.com/nogamsung/claude-ops/issues/12)) ([#22](https://github.com/nogamsung/claude-ops/issues/22)) ([ab74bd0](https://github.com/nogamsung/claude-ops/commit/ab74bd04e7aa78c7a0c7931156573ade9c5f1598))
* **github:** add webhook receiver for issue events ([#26](https://github.com/nogamsung/claude-ops/issues/26)) ([99e2535](https://github.com/nogamsung/claude-ops/commit/99e2535642ed8cd4e356785ff8feaaa119aa0ea5))
* **observability:** Prometheus /metrics + /metrics/forecast ([#19](https://github.com/nogamsung/claude-ops/issues/19)) ([21dbbbe](https://github.com/nogamsung/claude-ops/commit/21dbbbeb98bd35740658752df3efacfd440fec02))
* **quality:** post-task quality gate before PR creation ([#20](https://github.com/nogamsung/claude-ops/issues/20)) ([190c7ce](https://github.com/nogamsung/claude-ops/commit/190c7cecff0721e685b41e935026f432d8a23ac9))
* **scheduler:** scheduled maintenance cron (closes [#13](https://github.com/nogamsung/claude-ops/issues/13)) ([#23](https://github.com/nogamsung/claude-ops/issues/23)) ([9ed64d8](https://github.com/nogamsung/claude-ops/commit/9ed64d873fff79967cb2b14f4785283b5d1e4965))
* **usage:** per-task cost tracking + /usage aggregation API ([#27](https://github.com/nogamsung/claude-ops/issues/27)) ([ffc5399](https://github.com/nogamsung/claude-ops/commit/ffc5399c12a9f680d69bc1c53e7dcab4547cc51e))


### Miscellaneous Chores

* **harness:** update Claude Code Starter to v1.18.0 + /init go ([#25](https://github.com/nogamsung/claude-ops/issues/25)) ([292d36e](https://github.com/nogamsung/claude-ops/commit/292d36e15b6a0697b17fa4468fbb42fae24c4a72))

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
