# AI-Driven Development Factory Service Specification

Status: Draft v1

Purpose: ClickUp上のタスクをステータス駆動で検知し、Claude Codeによる仕様作成・コード実装を自律的に実行するオーケストレーションサービスを定義する。

## 1. Problem Statement

本サービスは、人間が「アイデア出し」と「レビュー」に専念し、AI（Claude Code）が「仕様の策定」から「コードの実装・PR作成」までを自律的に行う開発ワークフローを実現する、常駐型オーケストレーションサービスである。

OpenAI Symphonyの「ステータス駆動型・常駐プロセスによる状態管理」の思想を踏襲しつつ、以下の運用上の問題を解決する:

- タスク実行を手動スクリプトではなく、再現可能なデーモンワークフローに変換する。
- ClickUp上のステータス変更を自動検知し、適切なフェーズ（仕様作成 or 実装）のエージェントを起動する。
- インメモリ状態管理により、外部データベース不要で二重実行を防止する。
- GitHub Actionsをエージェント実行環境として使用し、低コストかつ堅牢な運用を実現する。

重要な境界:

- 本サービスはスケジューラ/ランナーであり、ClickUpのリーダーである。
- タスクの書き込み（ステータス遷移、PR URL設定等）はGitHub Actions上のエージェントが実行する。
- 成功した実行は「次のハンドオフ状態」（例: `Spec Review`, `PR Review`）で終了し、必ずしも `CLOSED` ではない。

## 2. Goals and Non-Goals

### 2.1 Goals

- 固定間隔（10秒）でClickUp APIをポーリングし、対象タスクをディスパッチする。
- インメモリ状態（`sync.RWMutex`）による単一権限の状態管理で、二重実行を防止する。
- ClickUpステータスを処理中に変更後、GitHub Actionsの `workflow_dispatch` をトリガーする。
- SPEC（仕様作成）とCODE（実装）の2フェーズをサポートする。
- 一時的な障害からの指数バックオフによるリトライ復旧。
- 構造化ログによるオペレータ向け可観測性の提供。
- 永続データベース不要のリスタート復旧をサポートする。

### 2.2 Non-Goals

- リッチなWeb UIやマルチテナント制御プレーン。
- 汎用ワークフローエンジンや分散ジョブスケジューラ。
- エージェント内部のビジネスロジック（PRの作成方法、コミットメッセージの決定等）。それらはGitHub Actionsワークフロー内のプロンプトとエージェントツールに委譲する。
- 強力なサンドボックス制御（GitHub Actionsランナーが提供する隔離に依存）。

## 3. System Overview

### 3.1 Main Components

1. **ClickUp Client** (Issue Tracker Adapter)
   - ClickUp APIを介して対象タスクを取得する。
   - ステータスフィルタリングにより候補タスクを選定する。
   - タスク情報を正規化された内部モデルに変換する。

2. **Orchestrator** (Core Scheduler)
   - ポーリングティックを所有する。
   - インメモリランタイム状態を管理する。
   - どのタスクをディスパッチ・リトライ・リリースするか決定する。

3. **GitHub Actions Dispatcher** (Agent Runner)
   - `workflow_dispatch` APIを介してGitHub Actionsワークフローをトリガーする。
   - タスクID、フェーズ（SPEC/CODE）を引数として渡す。

4. **Logging**
   - 構造化ランタイムログを出力する。

### 3.2 Abstraction Levels

1. **Coordination Layer** (Orchestrator)
   - ポーリングループ、タスク適格性判定、並行性制御、リトライ、リコンシリエーション。

2. **Execution Layer** (GitHub Actions Dispatcher)
   - `workflow_dispatch` トリガー、フェーズ別プロンプト切り替え。

3. **Integration Layer** (ClickUp Adapter)
   - ClickUp APIコールとタスクデータの正規化。

4. **Observability Layer** (Logs)
   - オペレータ向けの可視性。

### 3.3 External Dependencies

- ClickUp API（タスク取得・ステータス読み取り）。
- GitHub API（`workflow_dispatch` トリガー）。
- GitHub Actions（Claude Code CLI実行環境）。
- Anthropic Claude Code CLI（エージェント実行）。

## 4. Core Domain Model

### 4.1 Entities

#### 4.1.1 Task (ClickUp Task)

オーケストレーションで使用する正規化タスクレコード。

Fields:

- `id` (string)
  - ClickUpのタスクID。
- `name` (string)
  - タスク名。
- `description` (string or null)
  - タスクの説明文。
- `status` (string)
  - 現在のClickUpステータス名。
- `custom_fields` (map)
  - `github_pr_url` (string or null): エージェントが作成したPRリンク。
  - `agent_error` (string or null): エージェントエラーログ。
- `date_created` (timestamp)
- `date_updated` (timestamp)

#### 4.1.2 Phase

エージェント実行の2つのフェーズ。

- `SPEC`: 仕様作成フェーズ。タスク内容を読み取り、Markdown仕様書を生成しClickUpのDescriptionを更新する。
- `CODE`: 実装フェーズ。仕様に基づきコード変更・テスト実行・PR作成を行う。

#### 4.1.3 Run Attempt

1タスクに対する1実行試行。

Fields:

- `task_id`
- `phase` (SPEC or CODE)
- `attempt` (integer, 1-based for retries)
- `started_at`
- `status` (pending, running, succeeded, failed)
- `error` (optional)

#### 4.1.4 Orchestrator Runtime State

オーケストレータが保持する単一権限のインメモリ状態。

Fields:

- `poll_interval_ms` (integer, default: `10000`)
- `running_tasks` (map `task_id -> RunningEntry`)
  - `sync.RWMutex` で保護される。
- `claimed` (set of task IDs)

```go
type AgentState struct {
    mu           sync.RWMutex
    runningTasks map[string]time.Time // Key: ClickUp Task ID
}
```

### 4.2 Stable Identifiers and Normalization Rules

- **Task ID**: ClickUpのタスクIDをそのまま使用。内部マップキーおよびAPI操作に使用。
- **Normalized Task Status**: ステータスは小文字化して比較する。

## 5. ClickUp Workflow Design (Board Definition)

### 5.1 Status Definition

ClickUpの専用リストに以下のカスタムステータスを設定し、エージェントをステータス駆動で制御する。

| 順序 | ステータス名 | 担当 | 役割・トリガー条件 |
|------|-------------|------|-------------------|
| 1 | Idea Draft | 人間 | 初期状態。思いつきやバグをラフに記述する。 |
| 2 | Ready for Spec | 人間 | **[AIトリガー]** AIに仕様化を依頼する。 |
| 3 | Generating Spec | AI | Goが検知・自動変更。Claudeが仕様書を作成中。 |
| 4 | Spec Review | 人間 | AIが作成した仕様を人間が確認・修正する。 |
| 5 | Ready for Code | 人間 | **[AIトリガー]** 確定した仕様をもとにAIに実装を依頼する。 |
| 6 | Implementing | AI | Goが検知・自動変更。Claudeがコードを書きPRを作成中。 |
| 7 | PR Review | 人間 | PR作成完了。人間がコードレビューを行いマージする。 |
| 8 | CLOSED | 人間 | すべての作業が完了。 |

### 5.2 Trigger States (Active States)

オーケストレータがディスパッチ対象とするステータス:

- `ready for spec` -> Phase: SPEC
- `ready for code` -> Phase: CODE

### 5.3 Processing States

オーケストレータがステータス変更する中間状態:

- `generating spec` (SPEC フェーズ処理中)
- `implementing` (CODE フェーズ処理中)

### 5.4 Terminal States

タスクがこれらの状態にある場合、オーケストレータはリソースを解放する:

- `closed`

### 5.5 Custom Fields

| フィールド名 | 型 | 用途 |
|-------------|------|------|
| GitHub PR URL | URL | エージェントが作成したPRリンクを自動挿入する。 |
| Agent Error | Text | 実行中にエラーが起きた場合、ログを吐き出す場所。 |

## 6. Configuration Specification

### 6.1 Environment Variables

| 変数名 | 必須 | 説明 |
|--------|------|------|
| `CLICKUP_API_TOKEN` | Yes | ClickUp APIトークン |
| `CLICKUP_LIST_ID` | Yes | 対象ClickUpリストID |
| `GITHUB_PAT` | Yes | GitHub Personal Access Token |
| `GITHUB_OWNER` | Yes | GitHubリポジトリオーナー |
| `GITHUB_REPO` | Yes | GitHubリポジトリ名 |
| `GITHUB_WORKFLOW_FILE` | No | ワークフローファイル名 (default: `agent.yml`) |
| `POLL_INTERVAL_MS` | No | ポーリング間隔ミリ秒 (default: `10000`) |

### 6.2 Startup Validation

サービス起動時に以下を検証する:

- `CLICKUP_API_TOKEN` が設定されている。
- `CLICKUP_LIST_ID` が設定されている。
- `GITHUB_PAT` が設定されている。
- `GITHUB_OWNER` が設定されている。
- `GITHUB_REPO` が設定されている。

いずれかが欠落している場合、起動を失敗させエラーを出力する。

## 7. Orchestration State Machine

オーケストレータはスケジューリング状態を変更する唯一のコンポーネントである。

### 7.1 Task Orchestration States

これはClickUpステータスとは別の、サービス内部のクレーム状態である。

1. **Unclaimed**
   - タスクは実行されておらず、リトライもスケジュールされていない。

2. **Claimed**
   - オーケストレータがタスクを予約し、重複ディスパッチを防止している。

3. **Running**
   - GitHub Actionsワークフローがトリガーされ、タスクが `running_tasks` マップに追跡されている。

4. **RetryQueued**
   - ワーカーは実行されていないが、リトライタイマーが存在する。

5. **Released**
   - タスクが終端状態に遷移、または非アクティブになったためクレームが解除された。

### 7.2 Run Attempt Lifecycle

1. `DetectingTask` - ポーリングでトリガーステータスのタスクを検知。
2. `ClaimingTask` - インメモリロックでタスクをクレーム。
3. `UpdatingStatus` - ClickUpステータスを処理中に変更。
4. `TriggeringWorkflow` - GitHub Actions `workflow_dispatch` をトリガー。
5. `WaitingCompletion` - ワークフロー完了を待機。
6. `Succeeded` - 正常完了。
7. `Failed` - エラー終了。

### 7.3 Transition Triggers

- **Poll Tick**
  - リコンシリエーション実行。
  - 候補タスク取得。
  - スロットが空いていればディスパッチ。

- **Workflow Completion (normal)**
  - `running_tasks` からエントリ削除。
  - クレーム解除。
  - ClickUpステータスはGitHub Actions内のエージェントが更新済み。

- **Workflow Completion (abnormal)**
  - `running_tasks` からエントリ削除。
  - 指数バックオフリトライをスケジュール。
  - ClickUpのAgent Errorフィールドにエラーを記録。

- **Retry Timer Fired**
  - 候補タスクを再取得し、再ディスパッチまたはクレーム解除。

### 7.4 Idempotency and Recovery Rules

- オーケストレータは状態変更を単一権限でシリアライズし、重複ディスパッチを防止する。
- `claimed` と `running_tasks` のチェックがワーカー起動前に必須。
- リコンシリエーションはディスパッチの前に毎ティック実行される。
- リスタート復旧はClickUp APIからの状態読み取りに基づく（永続DBは不要）。

## 8. Polling, Scheduling, and Reconciliation

### 8.1 Poll Loop

起動時にコンフィグを検証し、即時ティックをスケジュールした後、`poll_interval_ms`（default: 10秒）ごとに繰り返す。

ティックシーケンス:

1. リコンシリエーション（実行中タスクのステータス確認）。
2. ClickUp APIで候補タスク取得（`Ready for Spec` または `Ready for Code` ステータス）。
3. 適格タスクをディスパッチ。
4. ログ出力。

### 8.2 Candidate Selection Rules

タスクがディスパッチ適格であるための条件:

- ステータスが `ready for spec` または `ready for code` である（小文字比較）。
- `running_tasks` に存在しない。
- `claimed` に存在しない。

### 8.3 Dispatch Flow

1. タスクIDを `claimed` に追加。
2. ClickUp APIでタスクステータスを処理中に変更:
   - `Ready for Spec` -> `Generating Spec`
   - `Ready for Code` -> `Implementing`
3. GitHub API経由で `workflow_dispatch` をトリガー:
   - `inputs.task_id` = ClickUpタスクID
   - `inputs.phase` = `SPEC` or `CODE`
4. タスクIDを `running_tasks` に追加。

### 8.4 Retry and Backoff

リトライエントリ作成:

- 同一タスクの既存リトライタイマーをキャンセル。
- `attempt`, `error`, `due_at_ms` を保存。

バックオフ計算:

- `delay = min(10000 * 2^(attempt - 1), 300000)`
- 最大バックオフ: 300秒（5分）。

リトライハンドリング:

1. 候補タスクを再取得。
2. 該当タスクが見つからなければクレーム解除。
3. タスクがまだ適格であればディスパッチ。
4. タスクが非アクティブであればクレーム解除。

### 8.5 Active Run Reconciliation

リコンシリエーションは毎ティック実行される。

- `running_tasks` 内の各タスクについて、ClickUp APIで現在のステータスを取得。
- ステータスが終端状態（`closed`）: ワーカーエントリ削除、クレーム解除。
- ステータスがまだアクティブ: 何もしない。
- ステータスがトリガー状態でも処理中状態でもない（人間が手動変更した場合等）: ワーカーエントリ削除、クレーム解除。
- ステータス取得失敗: ワーカーを維持し次のティックで再試行。

## 9. GitHub Actions Design (Agent Execution Environment)

### 9.1 Workflow File

リポジトリ内の `.github/workflows/agent.yml` に定義する。

### 9.2 Workflow Dispatch Inputs

```yaml
on:
  workflow_dispatch:
    inputs:
      task_id:
        description: 'ClickUp Task ID'
        required: true
        type: string
      phase:
        description: 'SPEC or CODE'
        required: true
        type: string
```

### 9.3 Phase A: SPEC (Specification Generation)

- **Input**: `task_id`, `phase=SPEC`
- **処理**:
  1. ClickUp APIからタスク内容を取得。
  2. リポジトリの現状（ディレクトリ構造、既存コード等）を読み取る。
  3. Claude Codeがタスク内容とコードベースを基にMarkdown仕様書を生成。
  4. ClickUp APIでタスクのDescriptionを仕様書で更新。
  5. ClickUp APIでステータスを `Spec Review` に変更。
- **Output**: タスクのDescriptionに仕様書が記載され、ステータスが `Spec Review` に遷移。

### 9.4 Phase B: CODE (Implementation)

- **Input**: `task_id`, `phase=CODE`
- **処理**:
  1. ClickUp APIからタスク内容（仕様書含む）を取得。
  2. `feature/clickup-{task_id}` ブランチを作成。
  3. Claude Codeが仕様を読み込み、コード変更を実施。
  4. テストを実行し、パスすることを確認。
  5. 変更をコミット＆Push。
  6. `gh pr create` でPRを作成。
  7. ClickUp APIでステータスを `PR Review` に変更。
  8. ClickUp APIでGitHub PR URLカスタムフィールドにPRリンクを設定。
- **Output**: PRが作成され、ステータスが `PR Review` に遷移、PRリンクがタスクに記録。

### 9.5 Error Handling in GitHub Actions

- エージェント実行中にエラーが発生した場合:
  1. ClickUp APIでAgent Errorカスタムフィールドにエラーメッセージを記録。
  2. ステータスはトリガーステータス（`Ready for Spec` or `Ready for Code`）に戻す。
  3. これによりオーケストレータの次のポーリングでリトライ対象となる。

## 10. Infrastructure and Deployment

### 10.1 Go Orchestrator Server

- **サーバー**: 安価なVPS（Linux）1台。
- **デプロイ**: Goアプリケーションをビルドし、単一バイナリとして配置。
- **デーモン化**: `systemd` でデーモン化し、24時間常駐稼働。
- **リスタート**: `systemd` の自動再起動設定でプロセスクラッシュからの復旧。

### 10.2 systemd Unit File (Example)

```ini
[Unit]
Description=ClickUp AI Workflow Tracker
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/clickup-ai-tracker
EnvironmentFile=/etc/clickup-ai-tracker/env
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 10.3 GitHub Actions Runner

- GitHub-hosted runnerを使用（追加インフラ不要）。
- Claude Code CLIはワークフロー内でセットアップ。
- 必要なシークレット:
  - `ANTHROPIC_API_KEY`: Claude Code CLI用。
  - `CLICKUP_API_TOKEN`: ClickUp API操作用。
  - `GITHUB_TOKEN`: 自動付与（PR作成用）。

## 11. Logging and Observability

### 11.1 Logging Conventions

タスク関連ログの必須コンテキストフィールド:

- `task_id`
- `phase`
- `status`

メッセージフォーマット:

- 安定した `key=value` フレージングを使用。
- アクション結果（`dispatched`, `completed`, `failed`, `retrying` 等）を含める。
- 障害時は簡潔な理由を含める。

### 11.2 Logging Outputs

- 標準エラー出力（stderr）に構造化ログを出力。
- `systemd` の `journalctl` で確認可能。

### 11.3 Key Log Events

| イベント | レベル | 説明 |
|---------|--------|------|
| `service_started` | INFO | サービス起動 |
| `poll_tick` | DEBUG | ポーリングティック実行 |
| `task_detected` | INFO | トリガーステータスのタスク検知 |
| `task_dispatched` | INFO | GitHub Actionsワークフロートリガー |
| `task_already_claimed` | WARN | 二重ディスパッチ防止 |
| `status_update_failed` | ERROR | ClickUpステータス変更失敗 |
| `workflow_trigger_failed` | ERROR | GitHub Actions トリガー失敗 |
| `reconciliation_release` | INFO | リコンシリエーションによるクレーム解除 |
| `retry_scheduled` | INFO | リトライスケジュール |
| `config_validation_failed` | FATAL | 起動時コンフィグ検証失敗 |

## 12. Project Structure

```
clickup-ai-workflow-tracker/
├── cmd/
│   └── server/
│       └── main.go            # エントリポイント
├── internal/
│   ├── config/
│   │   └── config.go          # 環境変数読み込み・バリデーション
│   ├── clickup/
│   │   ├── client.go          # ClickUp APIクライアント
│   │   └── models.go          # ClickUpタスクモデル
│   ├── github/
│   │   └── dispatcher.go      # GitHub Actions workflow_dispatch
│   ├── orchestrator/
│   │   ├── orchestrator.go    # ポーリングループ・ディスパッチロジック
│   │   └── state.go           # インメモリ状態管理 (AgentState)
│   └── logging/
│       └── logger.go          # 構造化ログ
├── .github/
│   └── workflows/
│       └── agent.yml          # Claude Code実行ワークフロー
├── go.mod
├── go.sum
├── SPEC.md                    # 本ドキュメント
└── .gitignore
```

## 13. Security Considerations

- **APIトークンの管理**: すべてのトークンは環境変数経由で注入し、コードにハードコードしない。
- **GitHub Actions シークレット**: `ANTHROPIC_API_KEY`, `CLICKUP_API_TOKEN` はGitHub Secretsに保存。
- **最小権限の原則**: `GITHUB_PAT` は `workflow_dispatch` と PR作成に必要な最小限のスコープで発行。
- **ClickUp APIトークン**: タスクの読み取りとステータス/フィールド更新に必要な権限のみ。

## 14. Notification Design

### 14.1 Notification Strategy

ClickUpのネイティブSlack連携機能を利用する。本サービス側で独自の通知ロジックを実装せず、ClickUp側の設定のみでSlack通知を実現する。

エージェントがClickUp APIでステータス変更やコメント追加を行うと、ClickUpのSlack連携が自動的に指定チャンネルへ通知を投稿する。

### 14.2 ClickUp Slack Integration Setup

ClickUp Settings > Integrations > Slack で以下を設定する:

- 対象ClickUpリスト（またはSpace/Folder）をSlackチャンネルに紐付ける。
- 通知対象イベントとして「ステータス変更」「コメント追加」を有効化する。

### 14.3 Notification Trigger Points

エージェント（GitHub Actions）がClickUp APIを操作することで、以下のタイミングでSlack通知が自動発火する。

| ClickUp操作 | Slack通知内容 |
|-------------|--------------|
| ステータスを `Spec Review` に変更 | 仕様書レビュー待ちになったことを通知 |
| ステータスを `PR Review` に変更 | コードレビュー待ちになったことを通知 |
| Agent Errorフィールド更新 | エラー発生を通知 |
| タスクコメント追加（PR URL等） | 補足情報の通知 |

### 14.4 Notification Flow Summary

```
[GitHub Actions] エージェント完了 or エラー発生
       |
       v
[ClickUp API] ステータス変更 + フィールド更新 + コメント追加
       |
       v
[ClickUp Slack Integration] 指定チャンネルへ自動通知
```

## 15. Future Considerations

以下は現時点ではスコープ外だが、将来的に検討する可能性がある:

- Webhook駆動（ポーリングからClickUp Webhookへの移行）。
- 複数リポジトリ対応。
- ダッシュボードUI（HTTP API経由でのランタイム状態公開）。
- タスク優先度に基づくディスパッチ順序制御。
- 並行エージェント実行の上限制御。
