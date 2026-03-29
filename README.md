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

新しいリポジトリにこのシステムを導入するには、以下の 2 つの設定が必要です。

### 1. ターゲットリポジトリの設定

`.github/workflows/agent.yaml` を対象リポジトリにコピーし、GitHub Secrets を設定します。

詳細は **[agent.yaml セットアップガイド](./docs/agent-setup-guide.md)** を参照してください。

### 2. オーケストレーターの設定

`projects.yaml` で ClickUp リストと GitHub リポジトリの対応を定義します。

```yaml
projects:
  - clickup_list_id: "XXXXXXXXX"       # ClickUp リスト ID（URL の /li/{list_id} から取得）
    github_owner: "your-org"
    github_repo: "your-repo"
    # github_workflow_file: "agent.yaml"  # optional (default: agent.yaml)
    # spec_output: "clickup"              # optional: "clickup" (default) or "repo"
    # status_mapping:                     # optional (defaults shown below)
    #   ready_for_spec: "ready for spec"
    #   generating_spec: "generating spec"
    #   spec_review: "spec review"
    #   ready_for_code: "ready for code"
    #   implementing: "implementing"
    #   pr_review: "pr review"
    #   closed: "closed"
```

### 3. 環境変数

必須の環境変数は `CLICKUP_API_TOKEN` と GitHub 認証（`GITHUB_PAT` または `GITHUB_APP_*`）の 2 つです。

| 変数名 | 必須 | 説明 |
|--------|------|------|
| `CLICKUP_API_TOKEN` | Yes | ClickUp API トークン |
| `GITHUB_PAT` | Yes (*1) | GitHub Personal Access Token（classic: `repo`, `workflow` / fine-grained: Contents + Actions の Read and write） |
| `GITHUB_APP_ID` | Yes (*1) | GitHub App ID |
| `GITHUB_APP_INSTALLATION_ID` | Yes (*1) | GitHub App Installation ID |
| `GITHUB_APP_PRIVATE_KEY` | Yes (*1) | GitHub App Private Key（base64 PEM。macOS: `base64 -i key.pem \| tr -d '\n'` / Linux: `base64 -w 0 < key.pem`） |
| `PROJECTS_FILE` | No | プロジェクト設定ファイルのパス（default: `projects.yaml`） |
| `POLL_INTERVAL_MS` | No | ポーリング間隔ミリ秒（default: `10000`） |
| `MAX_CONCURRENT_TASKS` | No | 並行タスク数上限（default: `0` = 無制限） |

*1: `GITHUB_PAT` と `GITHUB_APP_*` は排他。いずれか一方を設定。

<details>
<summary>ClickUp ステータス名のカスタマイズ（オプション）</summary>

既存の ClickUp リストのステータス名をそのまま使いたい場合、`projects.yaml` の `status_mapping` でプロジェクトごとにカスタマイズできます。省略したフィールドはデフォルト値が使われます。

```yaml
projects:
  - clickup_list_id: "XXXXXXXXX"
    github_owner: "your-org"
    github_repo: "your-repo"
    status_mapping:
      ready_for_spec: "spec待ち"
      generating_spec: "spec作成中"
      # 省略したフィールドはデフォルト値を使用
```

| フィールド | デフォルト値 |
|--------|-------------|
| `ready_for_spec` | `ready for spec` |
| `generating_spec` | `generating spec` |
| `spec_review` | `spec review` |
| `ready_for_code` | `ready for code` |
| `implementing` | `implementing` |
| `pr_review` | `pr review` |
| `closed` | `closed` |

</details>

<details>
<summary>SPEC フェーズの出力先カスタマイズ（オプション）</summary>

`spec_output` でプロジェクトごとに SPEC フェーズの出力先を選択できます。

| 値 | 動作 |
|------|------|
| `clickup`（default） | 仕様書を ClickUp タスクの Description に書き戻す |
| `repo` | 仕様書をリポジトリにコミットし、設計 PR を作成する。配置先やフォーマットは作業対象リポジトリの CLAUDE.md やスキル（`/spec` 等）に従う |

```yaml
projects:
  - clickup_list_id: "XXXXXXXXX"
    github_owner: "your-org"
    github_repo: "your-repo"
    spec_output: "repo"
```

</details>

### 4. オーケストレーターの起動

`.env` ファイルに環境変数を設定し、以下のいずれかで起動します。

**Docker（推奨）:**

```bash
docker run --rm \
  --env-file .env \
  -e PROJECTS_FILE=/projects.yaml \
  -v "$(pwd)/projects.yaml":/projects.yaml:ro \
  ghcr.io/alanse-inc/clickup-ai-orchestrator:latest
```

<details>
<summary>projects.yaml をイメージに埋め込む場合</summary>

```dockerfile
FROM ghcr.io/alanse-inc/clickup-ai-orchestrator:latest
COPY projects.yaml /projects.yaml
ENV PROJECTS_FILE=/projects.yaml
```

```bash
docker build -t my-orchestrator .
docker run --rm --env-file .env my-orchestrator
```
</details>

**ローカル実行:**

```bash
export $(grep -v '^#' .env | xargs)
go build -o bin/server ./cmd/server
./bin/server
```

## Usage

### 通常フロー（SPEC → CODE）

1. ClickUp のカンバンでタスクを **「Idea Draft」** に作成し、概要を記述する
2. タスクを **「Ready for Spec」** に移動する → AI が仕様書を自動作成
3. **「Spec Review」** で仕様を確認・修正し、**「Ready for Code」** に移動する → AI がコードを実装し PR を作成
4. **「PR Review」** でコードレビュー・マージする → オーケストレータが PR のマージを検知し、タスクを自動で **「CLOSED」** に遷移

### ショートカットフロー（SPEC スキップ）

軽微なバグ修正やリファクタリングなど仕様策定が不要なタスクでは、SPEC フェーズをスキップできます。

1. ClickUp のカンバンでタスクを **「Idea Draft」** に作成し、やりたいことを記述する
2. タスクを直接 **「Ready for Code」** に移動する → AI がタスク名と説明をもとにコードを実装し PR を作成
3. **「PR Review」** でコードレビュー・マージする → オーケストレータが PR のマージを検知し、タスクを自動で **「CLOSED」** に遷移

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
