# Changelog

## [1.0.4](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.0.3...v1.0.4) (2026-03-26)


### Bug Fixes

* Dockerfile に ca-certificates を追加して TLS エラーを修正 ([#54](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/54)) ([698acf2](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/698acf2373e4a8daf7d8a96e62593b9ad4b1eb5d))

## [1.0.3](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.0.2...v1.0.3) (2026-03-21)


### Bug Fixes

* add skip-labeling to release-please config ([#52](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/52)) ([a0049d5](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/a0049d5c7bba01664aee8591b36136d27b251e9f))

## [1.0.2](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.0.1...v1.0.2) (2026-03-21)


### Bug Fixes

* add release-please manifest to fix changelog duplication ([#48](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/48)) ([440985f](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/440985fbe9a63568961b8635f1b5f1f031480d56))

## [1.0.1](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.0.0...v1.0.1) (2026-03-21)


### Bug Fixes

* move Docker push into release-please workflow ([#45](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/45)) ([bfa9f43](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/bfa9f430f0ada3a9453e177dcbd58304abf23ca5))

## 1.0.0 (2026-03-17)


### Features

* add apply-issue skill for end-to-end issue workflow ([#12](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/12)) ([29eafa2](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/29eafa2ab227884c9b3bd8c529d8db988380aeb7))
* add ClickUp description update, PR URL field, and error recording to agent.yml ([#16](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/16)) ([2240270](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/2240270732f218375a64a077f3d62043275518ae)), closes [#3](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/3)
* add Docker image release workflow and document Docker setup ([#42](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/42)) ([439f6fc](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/439f6fc16570e7bcc177dccfd3ed48c1b2ea6bbf))
* add Dockerfile and deployment configuration ([#24](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/24)) ([e5e0a87](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/e5e0a87b680a310d6b08953d0c3a135f8a9b72ad)), closes [#17](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/17)
* add GitHub App (Installation Token) authentication support ([#15](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/15)) ([c5a50c4](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/c5a50c47160e787e249da9820c3e082d28e3791a)), closes [#10](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/10)
* add health check endpoint, projects.yaml example, and update README ([#40](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/40)) ([b010bcc](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/b010bcca3d3c4cdab21bf9966776bcd5ccf34bcb))
* add MAX_CONCURRENT_TASKS to limit parallel agent dispatches ([#30](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/30)) ([e3bbcce](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/e3bbcce68f43da2cdbd2311a93e3040a77a7ae6c))
* add shortcut flow to skip SPEC phase for simple tasks ([#29](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/29)) ([a114963](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/a114963f2636276016fb039a6105aa8244e36d0a)), closes [#21](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/21)
* GITHUB_APP_PRIVATE_KEY の base64 エンコード対応 ([#27](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/27)) ([8fdad84](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/8fdad846d6efc7c0886435e141ffc5250ce5b314))
* implement orchestrator service with all core components ([#9](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/9)) ([51156d1](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/51156d15a4bb19adcf78fe9c4c87ba9ca3becec3))
* make ClickUp status names configurable and activate logging package ([#14](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/14)) ([25fa2bc](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/25fa2bcad2f63714be0c5da80bd783643337caf4))
* replace PR URL custom field with ClickUp GitHub integration ([#28](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/28)) ([2c60294](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/2c6029440f243741132ae6f3e647dff8a1ec8600)), closes [#18](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/18)
* 複数リポジトリ対応 ([#39](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/39)) ([c8f89c2](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/c8f89c24b3f528da3d1a2b90a0273dfbad914c0a)), closes [#19](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/19)


### Bug Fixes

* add issues: write permission to release-please workflow ([#43](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/43)) ([31f25ce](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/31f25cea2b16e1998801beabac25075515e2f154))
