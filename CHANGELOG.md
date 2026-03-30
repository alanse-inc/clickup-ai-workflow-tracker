# Changelog

## [1.6.0](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.5.0...v1.6.0) (2026-03-30)


### Features

* リリース時に deployment リポジトリへ自動デプロイトリガー ([#95](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/95)) ([87ca4fc](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/87ca4fcce7e6993fe1e7b75f6e28069abcb81277))

## [1.5.0](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.4.0...v1.5.0) (2026-03-29)


### Features

* migrate POLL_INTERVAL_MS/MAX_CONCURRENT_TASKS/SHUTDOWN_TIMEOUT_MS to projects.yaml ([#92](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/92)) ([739e346](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/739e346bf58dc9d9ee6b01cd3a23f3413798c4ba))
* SPEC PR マージ時に自動で READY FOR CODE へステータス遷移 ([#90](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/90)) ([2b10c01](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/2b10c017f51e05cb36bed506bb357b575217c0a7))
* プロジェクト設定の部分的エラー耐性 - 不正な設定をスキップして稼働継続 ([#91](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/91)) ([cbfefc7](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/cbfefc703f15203b86d27b2dad58fbb06719bd64))


### Bug Fixes

* repo モードの SPEC PR 存在確認と PR チェッカーのエラー伝播を修正 ([#94](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/94)) ([c9afbff](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/c9afbff43fa082ea112835f493cfb53f02016b76))

## [1.4.0](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.3.0...v1.4.0) (2026-03-29)


### Features

* manage ClickUp statuses per project via YAML config ([#85](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/85)) ([301516b](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/301516bc3d1e8e8ca630e73cc12d11a7a69cf71a))
* self-develop スキルを追加 ([#89](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/89)) ([02a650b](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/02a650b9db984eb23e19c3bbae09f44db51e34d1))
* SPEC フェーズのリポジトリベース出力モード対応 ([#88](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/88)) ([db8b45d](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/db8b45dc470680d0fa2567e6a42f8e99b0adc486))

## [1.3.0](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.2.0...v1.3.0) (2026-03-28)


### Features

* /status エンドポイントを追加してランタイム状態を外部公開 ([#78](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/78)) ([fa3b371](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/fa3b3718d69dab593d924c743a4afe14ccf9abc2))
* agent.yaml phase 入力を choice 型にしてバリデーション追加 ([#76](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/76)) ([cb02868](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/cb02868f104828b917b25d427b6eb545e233d23c))
* Go 品質チェック (fmt/lint/test) を hooks・skills・agent に統合 ([#82](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/82)) ([cdef0bb](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/cdef0bbf27769488a36cd08e0046918a0cc66493))
* graceful shutdown で実行中 dispatch の完了を待機する ([#72](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/72)) ([c0228b5](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/c0228b506fbc8a8301cb4829a8fb9386ff2fcb45))
* PR マージ時に ClickUp タスクを自動で CLOSED に遷移する ([#83](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/83)) ([a085600](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/a085600efd6cdcc18f0638657a53295fcccb8011))
* ヘルスチェックを全プロジェクト対応にする ([#71](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/71)) ([ab8efce](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/ab8efcea8e7a5d9d753a07b8174df01ed9fbe2a9))
* 再起動後の processing ステータスタスク復旧機能を実装 ([#74](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/74)) ([5e89f14](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/5e89f140653d79042754a551880dfcbda2ab2aac))


### Bug Fixes

* claude-code-review に allowed_bots を追加 ([#77](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/77)) ([174789f](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/174789f0bb5c2bd86f19fae670f0da443ed1873f))
* execution_file の構造をデバッグ出力し再帰的テキスト抽出に対応 ([#80](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/80)) ([834e3e8](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/834e3e89ce15dddcf24934129fe2f8079d3a76c2))
* SPEC フェーズで元の description を保持し markdown_description で追記する ([#65](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/65)) ([2a1fc84](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/2a1fc84780d3df3b738e120132863c9a5064442c))
* SPEC フェーズの仕様書を execution_file から抽出する ([#79](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/79)) ([454c986](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/454c9868f0ee3f9631ae4f33de070e67be9524b7))
* SPEC フェーズの仕様書抽出ロジックを堅牢にする ([#75](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/75)) ([1a51fd8](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/1a51fd86220f67e5c978d7e290bf45b6ac911dbe))

## [1.2.0](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.1.0...v1.2.0) (2026-03-27)


### Features

* ClickUp API GetTasks のページネーション対応 ([#64](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/64)) ([c3f5a48](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/c3f5a48dd104f0e29cfae0d34c293320a9379ed9))


### Bug Fixes

* agent.yaml の GitHub App シークレット名を CI_APP_* に変更 ([1d9a855](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/1d9a85595b122f90fce99ff257d94d28f80b6ff4))
* Claude Code Review で Bot PR を許可する allowed_bots を追加 ([df2f98c](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/df2f98cb3fc973ea75f1e4c1aca01f8aa5769f45))
* ClickUp API クライアントのエラーレスポンスにボディを含める ([#62](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/62)) ([0443232](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/0443232d3f64bcdfd4db357af6b2b66a7014ee6a))

## [1.1.0](https://github.com/alanse-inc/clickup-ai-orchestrator/compare/v1.0.4...v1.1.0) (2026-03-27)


### Features

* CODE フェーズで ClickUp に PR リンクをコメント投稿 & 導入ガイド追加 ([928db83](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/928db83935842405355755c8912dbc37913978b8))
* ヘルスチェックエンドポイントに依存サービスの接続状態を追加 ([#58](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/58)) ([fc8a1e3](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/fc8a1e399fdaadd1ae2ae94a4f9f0eba69faf33c))


### Bug Fixes

* agent ジョブに contents:write, pull-requests:write 権限を追加 ([5d07503](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/5d07503a9f3023c4425c1dbe8aafc317bef9ae99))
* agent.yaml の認証を claude_code_oauth_token に変更 ([9f982a8](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/9f982a81287eac2638ea89874cde3895a3d08b4e))
* claude-code-action に allowed_bots を追加 ([0278c1d](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/0278c1dabe19822b75b3d0aa68da2dc0d78deb1d))
* CODE フェーズに additional_permissions を追加 ([2b8b5d0](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/2b8b5d02d61f600ab6873b4bbd9888080b27193a))
* CODE フェーズに allowed_tools を追加 ([1ef11d2](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/1ef11d2768205bf63a415967a4ec65bf62a5aab1))
* CODE フェーズの権限を settings パラメータで設定 ([6ce143c](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/6ce143cad99293d4504c4ea38aff6447e5cef570))
* extract-spec-result.py を JSON 配列形式にも対応 ([ac2e659](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/ac2e6599c18f91adadcee5f59cbc06d6646dc3ad))
* GitHub App トークンで Bot PR の CI 自動トリガーをサポート ([#59](https://github.com/alanse-inc/clickup-ai-orchestrator/issues/59)) ([d2ec3de](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/d2ec3deff004cdadca4dd2bb53d1aad5bad5205b))
* SPEC フェーズに show_full_output を追加 ([2a98857](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/2a9885731c46ca4c652a4c303f70a948d63aee3a))
* SPEC 結果を実行ログファイルから抽出して ClickUp Description に書き込む ([d8afe54](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/d8afe54795cf1075ea319e75fb0b43cc126a6248))
* SPEC 結果抽出の jq フィルタを修正 ([6cfdf97](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/6cfdf975aab9edaee0438ce3a03e33e4357ce97e))
* SPEC 結果抽出を python3 に変更しファイル経由で ClickUp に送信 ([9dd17bf](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/9dd17bf9d9f95935a846ec736b493dc9e7f1c3ad))
* SPEC 結果抽出を外部スクリプトに分離 ([5c9b2ee](https://github.com/alanse-inc/clickup-ai-orchestrator/commit/5c9b2ee8161ee0d7722918bb87dcdb21f9e8d091))

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
