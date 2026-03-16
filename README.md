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
- **`agent_error`** (テキスト型): エージェント実行エラーのメッセージを記録

> **Note**: PR の ClickUp タスクへのリンクは ClickUp の GitHub インテグレーション（GitHub App）で自動処理されます。ClickUp ワークスペースの設定から GitHub App をインストールしておいてください。

### 2. ターゲットリポジトリへの agent.yaml 配置

本リポジトリの `.github/workflows/agent.yaml` は、**ターゲットリポジトリ（Claude Code が実装を行うリポジトリ）に配置するテンプレート**です。オーケストレーターは `workflow_dispatch` API でターゲットリポジトリのワークフローを起動するため、このファイルをターゲットリポジトリの `.github/workflows/agent.yaml` にコピーしてください。

ターゲットリポジトリの GitHub Secrets に以下を設定してください:

| Secret 名 | 説明 |
|-----------|------|
| `ANTHROPIC_API_KEY` | Claude API キー |
| `CLICKUP_API_TOKEN` | ClickUp API トークン（ステータス更新用） |
| `CLICKUP_AGENT_ERROR_FIELD_ID` | `agent_error` カスタムフィールドの ID（ワークフロー失敗時のエラー記録に使用） |

`CLICKUP_AGENT_ERROR_FIELD_ID` の値は ClickUp API で取得できます:

```bash
curl -s "https://api.clickup.com/api/v2/list/{list_id}/field" \
  -H "Authorization: <CLICKUP_API_TOKEN>" \
  | jq '.fields[] | select(.name == "agent_error") | .id'
```

### 3. プロジェクト設定ファイル（projects.yaml）

`projects.yaml.example` をコピーして `projects.yaml` を作成し、ClickUp リストと GitHub リポジトリの対応を設定してください。

```bash
cp projects.yaml.example projects.yaml
```

```yaml
projects:
  - clickup_list_id: "XXXXXXXXX"
    github_owner: "your-org"
    github_repo: "your-repo"
    # github_workflow_file: "agent.yaml"  # optional (default: agent.yaml)
```

複数プロジェクトを管理する場合は、`projects` 配列に複数エントリを追加してください。

### 4. 環境変数

| 変数名 | 必須 | 説明 |
|--------|------|------|
| `CLICKUP_API_TOKEN` | Yes | ClickUp API トークン |
| `GITHUB_PAT` | Yes (*1) | GitHub Personal Access Token（classic: `repo`, `workflow` スコープ / fine-grained: Contents + Actions の Read and write） |
| `GITHUB_APP_ID` | Yes (*1) | GitHub App ID |
| `GITHUB_APP_INSTALLATION_ID` | Yes (*1) | GitHub App Installation ID |
| `GITHUB_APP_PRIVATE_KEY` | Yes (*1) | GitHub App Private Key（base64 エンコードした PEM）macOS: `base64 -i key.pem \| tr -d '\n'` / Linux: `base64 -w 0 < key.pem` |
| `PROJECTS_FILE` | No | プロジェクト設定ファイルのパス（default: `projects.yaml`） |
| `POLL_INTERVAL_MS` | No | ポーリング間隔ミリ秒（default: `10000`） |
| `MAX_CONCURRENT_TASKS` | No | 並行タスク数上限（default: `0` = 無制限） |

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

### 5. オーケストレーターの起動

#### ローカル実行

```bash
# .env を環境変数として読み込み
export $(grep -v '^#' .env | xargs)

go build -o bin/server ./cmd/server
./bin/server
```

#### Docker

```bash
docker build -t clickup-ai-orchestrator .
docker run --env-file .env clickup-ai-orchestrator
```

> **Note**: Docker の `--env-file` はマルチライン値を扱えないため、GitHub App 認証（`GITHUB_APP_PRIVATE_KEY`）を使用する場合は base64 エンコードした値を設定してください。

## Usage

### 通常フロー（SPEC → CODE）

1. ClickUp のカンバンでタスクを **「Idea Draft」** に作成し、概要を記述する
2. タスクを **「Ready for Spec」** に移動する → AI が仕様書を自動作成
3. **「Spec Review」** で仕様を確認・修正し、**「Ready for Code」** に移動する → AI がコードを実装し PR を作成
4. **「PR Review」** でコードレビュー・マージする

### ショートカットフロー（SPEC スキップ）

軽微なバグ修正やリファクタリングなど仕様策定が不要なタスクでは、SPEC フェーズをスキップできます。

1. ClickUp のカンバンでタスクを **「Idea Draft」** に作成し、やりたいことを記述する
2. タスクを直接 **「Ready for Code」** に移動する → AI がタスク名と説明をもとにコードを実装し PR を作成
3. **「PR Review」** でコードレビュー・マージする

> **Tip**: ステータスの移動は ClickUp カンバンボード上でドラッグ＆ドロップするだけです。オーケストレータが 10 秒間隔でポーリングし、自動的に検知・ディスパッチします。

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
