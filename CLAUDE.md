# Project: clickup-ai-workflow-tracker

ClickUp上のタスクをステータス駆動で検知し、GitHub Actions経由でClaude Codeを起動して仕様作成・コード実装を自律的に実行するGoオーケストレーションサービス。

## Architecture

詳細は SPEC.md を参照。

- **Go Server (Orchestrator)**: 10秒間隔でClickUp APIをポーリングし、対象タスクを検知してGitHub Actionsをトリガーする常駐プロセス
- **GitHub Actions (Agent)**: Claude Code CLIが仕様作成(SPEC)または実装(CODE)を実行
- **ClickUp**: タスク管理・ステータス駆動の制御盤

## Directory Structure

```
cmd/server/main.go          - エントリポイント
internal/config/             - 環境変数読み込み・バリデーション
internal/clickup/            - ClickUp APIクライアント・タスクモデル
internal/github/             - GitHub Actions workflow_dispatch
internal/orchestrator/       - ポーリングループ・ディスパッチ・インメモリ状態管理
internal/logging/            - 構造化ログ
.github/workflows/agent.yml - Claude Code実行ワークフロー
```

## Commands

```bash
go build -o bin/server ./cmd/server  # ビルド
go test ./...                        # テスト
golangci-lint run ./...              # Lint
golangci-lint fmt ./...              # Format
go vet ./...                         # 静的解析
```

golangci-lint はプロジェクトの Go 依存ではなく、グローバルにインストールする開発ツール。
`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` でインストールする。
CI では golangci-lint-action が自動でバイナリを取得するため go.mod への追加は不要。

## Environment Variables

| 変数名 | 必須 | 説明 |
|--------|------|------|
| CLICKUP_API_TOKEN | Yes | ClickUp APIトークン |
| CLICKUP_LIST_ID | Yes | 対象ClickUpリストID |
| GITHUB_PAT | Yes | GitHub Personal Access Token |
| GITHUB_OWNER | Yes | GitHubリポジトリオーナー |
| GITHUB_REPO | Yes | GitHubリポジトリ名 |
| GITHUB_WORKFLOW_FILE | No | ワークフローファイル名 (default: agent.yml) |
| POLL_INTERVAL_MS | No | ポーリング間隔ミリ秒 (default: 10000) |

## Coding Conventions

- Go標準のコーディング規約に従う
- エラーハンドリングはシステム境界（外部API呼び出し）にのみ行う
- パッケージは internal/ 配下に配置し、外部公開しない
- テストは *_test.go に記述し、テーブル駆動テストを基本とする
- ログは構造化ログ（JSON形式、slog.NewJSONHandler）を使用する

## Development Flow

詳細は DEVELOPMENT.md を参照。

- GitHub Issues でタスク管理
- 1セッション1タスクの原則
- TDD: テストを先に書き、実装はテストが通る最小限に
- コミットメッセージ: `<type>: <summary>` (feat/fix/refactor/test/docs/chore)
- ブランチ: `feature/<issue-number>-<short-desc>` or `fix/<issue-number>-<short-desc>`
