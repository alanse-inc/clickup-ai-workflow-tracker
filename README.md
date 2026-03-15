# clickup-ai-orchestrator

ClickUp 上のタスクをステータス駆動で検知し、GitHub Actions 経由で Claude Code を起動して仕様作成・コード実装を自律的に実行する Go オーケストレーションサービス。

## Why Go?

alanse では TypeScript を主軸に開発していますが、本プロジェクトでは Go を採用しています。

**Goroutine によるシンプルな並行処理**: ポーリングループ、タスクディスパッチ、ステータス監視といった複数の非同期処理を Goroutine と `sync.Mutex` で自然に記述できます。状態管理のためにデータベースや外部キューを持つ必要がなく、インメモリで完結します。

**シングルバイナリ・省リソース**: ビルド成果物は単一バイナリ。安価な VPS に配置して `systemd` でデーモン化するだけで 24 時間稼働します。Node.js ランタイムや依存のインストールが不要で、メモリ消費も極めて少ないです。

**Symphony との対比**: OpenAI Symphony（Elixir 実装）は Linear ボードを監視しタスクごとに隔離された自律的な実行環境を生成する設計ですが、本システムは ClickUp をタスク管理に、GitHub Actions にエージェント実行を委譲するため、オーケストレータ自体は軽量なスケジューラに徹します。Go のシンプルさと低フットプリントがこの役割に適しています。

## Architecture

```
[ClickUp] ── polling ──> [Go Server] ── workflow_dispatch ──> [GitHub Actions + Claude Code]
                              |                                         |
                              |                                         v
                              +<──────── ClickUp API ──────────── status update
                                                                        |
                                                                        v
                                                                  [Slack notification]
```

詳細は [SPEC.md](./SPEC.md) を参照してください。

## Setup

> **Note**: 本プロジェクトはまだ開発途中です。

### 1. ClickUp ボードの準備

対象リストに以下の 8 ステータスを順番通りに作成してください:

`idea draft` → `ready for spec` → `generating spec` → `spec review` → `ready for code` → `implementing` → `pr review` → `closed`

また、以下のカスタムフィールドを作成してください:
- **`github_pr_url`** (URL 型): PR の URL を記録
- **`agent_error`** (テキスト型): エラーメッセージを記録

### 2. ターゲットリポジトリへの agent.yml 配置

本リポジトリの `.github/workflows/agent.yml` は、**ターゲットリポジトリ（Claude Code が実装を行うリポジトリ）に配置するテンプレート**です。オーケストレーターは `workflow_dispatch` API でターゲットリポジトリのワークフローを起動するため、このファイルをターゲットリポジトリの `.github/workflows/agent.yml` にコピーしてください。

ターゲットリポジトリの GitHub Secrets に以下を設定してください:
- `ANTHROPIC_API_KEY`: Claude API キー
- `CLICKUP_API_TOKEN`: ClickUp API トークン（ステータス更新用）

### 3. 環境変数

`.env.example` をコピーして `.env` を作成し、値を設定してください。

```bash
cp .env.example .env
```

| 変数名 | 必須 | 説明 |
|--------|------|------|
| `CLICKUP_API_TOKEN` | Yes | ClickUp API トークン |
| `CLICKUP_LIST_ID` | Yes | 対象 ClickUp リスト ID |
| `GITHUB_PAT` | Yes (*1) | GitHub Personal Access Token（classic: `repo`, `workflow` スコープ / fine-grained: Contents + Actions の Read and write） |
| `GITHUB_APP_ID` | Yes (*1) | GitHub App ID |
| `GITHUB_APP_INSTALLATION_ID` | Yes (*1) | GitHub App Installation ID |
| `GITHUB_APP_PRIVATE_KEY` | Yes (*1) | GitHub App Private Key（base64 エンコードした PEM）macOS: `base64 -i key.pem | tr -d '\n'` / Linux: `base64 -w 0 < key.pem` |
| `GITHUB_OWNER` | Yes | ターゲットリポジトリのオーナー |
| `GITHUB_REPO` | Yes | ターゲットリポジトリ名 |
| `GITHUB_WORKFLOW_FILE` | No | ワークフローファイル名（default: `agent.yml`） |
| `POLL_INTERVAL_MS` | No | ポーリング間隔ミリ秒（default: `10000`） |

*1: `GITHUB_PAT` と `GITHUB_APP_*` は排他。いずれか一方を設定してください。

<details>
<summary>ClickUp ステータス名のカスタマイズ（オプション）</summary>

| 変数名 | デフォルト値 |
|--------|-------------|
| `CLICKUP_STATUS_READY_FOR_SPEC` | `ready for spec` |
| `CLICKUP_STATUS_GENERATING_SPEC` | `generating spec` |
| `CLICKUP_STATUS_SPEC_REVIEW` | `spec review` |
| `CLICKUP_STATUS_READY_FOR_CODE` | `ready for code` |
| `CLICKUP_STATUS_IMPLEMENTING` | `implementing` |
| `CLICKUP_STATUS_PR_REVIEW` | `pr review` |
| `CLICKUP_STATUS_CLOSED` | `closed` |

</details>

### 4. オーケストレーターの起動

#### Docker

```bash
docker build -t clickup-tracker .
docker run --env-file .env clickup-tracker
```

> **Note**: Docker の `--env-file` はマルチライン値を扱えないため、GitHub App 認証（`GITHUB_APP_PRIVATE_KEY`）を使用する場合は base64 エンコードした値を設定してください。

#### ローカル実行

```bash
# .env を環境変数として読み込み
export $(grep -v '^#' .env | xargs)

go build -o bin/server ./cmd/server
./bin/server
```

## Development

開発フローについては [DEVELOPMENT.md](./DEVELOPMENT.md) を参照してください。

```bash
go build -o bin/server ./cmd/server  # ビルド
go test ./...                        # テスト
golangci-lint run ./...              # Lint
```

> **Note**: `golangci-lint` はプロジェクトの Go 依存ではなく、グローバルにインストールする開発ツールです。
> `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` でインストールしてください。
> CI では `golangci-lint-action` が自動でバイナリを取得するため `go.mod` への追加は不要です。
