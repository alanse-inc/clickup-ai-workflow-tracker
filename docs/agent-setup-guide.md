# agent.yaml セットアップガイド

このガイドでは、clickup-ai-orchestrator の `agent.yaml` を新しいリポジトリに導入し、ClickUp タスク駆動で Claude Code による仕様作成・コード実装を自動化する手順を説明します。

## 1. 前提条件

- ClickUp ワークスペースに **GitHub Integration**（GitHub App）がインストール済みであること
  - ClickUp Settings > Integrations > GitHub からインストールできます
  - PR とタスクの自動リンクに使用されます
- 対象リポジトリへの管理者権限（GitHub Secrets の設定に必要）
- `clickup-ai-orchestrator-deployment` リポジトリへのアクセス権（オーケストレーター設定の変更に必要）

## 2. agent.yaml のコピーと配置

本リポジトリの `.github/workflows/agent.yaml` を、対象リポジトリの `.github/workflows/agent.yaml` にそのままコピーしてください。

```bash
# 例: 対象リポジトリのルートで実行
mkdir -p .github/workflows
cp /path/to/clickup-ai-orchestrator/.github/workflows/agent.yaml .github/workflows/agent.yaml
```

ファイルの内容は変更不要です。agent.yaml はポータブルに設計されており、ステータス名などの設定値はオーケストレーターから `workflow_dispatch` の入力パラメータとして渡されます。

## 3. GitHub Secrets の設定

対象リポジトリの Settings > Secrets and variables > Actions で以下の Secrets を設定します。

### 3.1 `CLICKUP_API_TOKEN`（必須）

ClickUp API トークンです。ワークフロー内でタスク情報の取得・ステータス更新に使用します。

**取得手順:**

1. ClickUp にログイン
2. 左下のアバター > **Settings** を開く
3. サイドバーの **Apps** をクリック
4. **API Token** セクションで **Generate** をクリック（既存トークンがある場合はそれを使用）
5. 表示されたトークンをコピー

> **Note**: API トークンはワークスペース単位ではなくユーザー単位で発行されます。Bot 用のサービスアカウントで発行することを推奨します。

### 3.2 `CLAUDE_CODE_OAUTH_TOKEN`（必須）

Claude Code Action の実行に必要な OAuth トークンです。

**設定手順:**

1. ターミナルで以下のコマンドを実行:
   ```bash
   claude /install-github-app
   ```
2. ブラウザが開き、GitHub App のインストールフローが始まります
3. 対象リポジトリを選択してインストール
4. 完了すると `CLAUDE_CODE_OAUTH_TOKEN` が自動的に GitHub Secrets に登録されます

### 3.3 `GITHUB_APP_ID` / `GITHUB_APP_PRIVATE_KEY`（推奨）

GitHub App のインストールトークンを生成するために使用します。設定すると、Claude Code が作成した PR で CI ワークフローが自動的にトリガーされます。

> **背景**: GitHub Actions のデフォルト `GITHUB_TOKEN` で push/PR 作成すると、セキュリティ上の理由で他のワークフロー（CI など）がトリガーされません。GitHub App トークンを使うことでこの制限を回避できます。

オーケストレーターが使用している GitHub App と同じものを流用できます。

**設定手順:**

1. オーケストレーターの環境変数に設定済みの `GITHUB_APP_ID` と `GITHUB_APP_PRIVATE_KEY` の値を取得
2. 対象リポジトリの GitHub Secrets に同じ値を登録

> **Tip**: Organization レベルの Secret として設定すると、全リポジトリで共有できます。未設定の場合は `GITHUB_TOKEN` にフォールバックしますが、CI は自動トリガーされません。

### 3.4 `CLICKUP_AGENT_ERROR_FIELD_ID`（オプション）

ワークフロー失敗時にエラーメッセージを ClickUp タスクのカスタムフィールドに記録するために使用します。設定しない場合、エラー記録ステップはスキップされます。

**取得手順:**

1. ClickUp の対象リストに **`agent_error`**（テキスト型）カスタムフィールドを作成
2. 以下のコマンドでフィールド ID を取得:
   ```bash
   curl -s "https://api.clickup.com/api/v2/list/{list_id}/field" \
     -H "Authorization: <CLICKUP_API_TOKEN>" \
     | jq '.fields[] | select(.name == "agent_error") | .id'
   ```
3. 取得した ID を GitHub Secrets に登録

## 4. オーケストレーター側の設定

### 4.1 ClickUp リストの作成

対象プロジェクト用の ClickUp リストを作成し、以下のステータスを順番通りに設定してください:

| 順序 | ステータス名 | 担当 | 説明 |
|------|-------------|------|------|
| 1 | `ready for spec` | 人間 | AI に仕様作成を依頼（トリガー） |
| 2 | `generating spec` | AI | 仕様書を生成中 |
| 3 | `spec review` | 人間 | 仕様書をレビュー |
| 4 | `ready for code` | 人間 | AI に実装を依頼（トリガー） |
| 5 | `implementing` | AI | コード実装・PR 作成中 |
| 6 | `pr review` | 人間 | PR をレビュー・マージ |
| 7 | `closed` | — | 完了（Closed カテゴリ） |

> **Note**: ステータス名は小文字で比較されます。表記は自由ですが、正規化後に上記の値と一致する必要があります。

### 4.2 projects.yaml にエントリを追加

`clickup-ai-orchestrator-deployment` リポジトリの `projects.yaml` に新しいエントリを追加します。

```yaml
projects:
  - clickup_list_id: "XXXXXXXXX"       # ClickUp リストの ID
    github_owner: "your-org"            # GitHub オーナー（Organization or User）
    github_repo: "your-repo"            # GitHub リポジトリ名
    # github_workflow_file: "agent.yaml"  # optional (default: agent.yaml)
```

ClickUp リスト ID は、ClickUp でリストを開いた際の URL から確認できます:
`https://app.clickup.com/{workspace_id}/v/li/{list_id}`

### 4.3 デプロイの実行

`projects.yaml` の変更を PR 経由でマージした後、deployment リポジトリの GitHub Actions で `Deploy to Cloud Run` ワークフローを手動実行（`workflow_dispatch`、tag=main）してください。

## 5. 動作確認

### 5.1 SPEC フェーズの確認

1. ClickUp で対象リストにタスクを作成し、タスク名と説明を記述
2. タスクのステータスを **「ready for spec」** に移動
3. 約 10 秒以内にオーケストレーターがタスクを検知し、ステータスが **「generating spec」** に自動変更される
4. 対象リポジトリの **Actions** タブで「AI Agent」ワークフローが実行されていることを確認
5. ワークフロー完了後、ClickUp タスクの Description に仕様書（Markdown）が書き込まれ、ステータスが **「spec review」** に遷移する

### 5.2 CODE フェーズの確認

1. 仕様書の内容を確認・必要に応じて修正
2. タスクのステータスを **「ready for code」** に移動
3. ステータスが **「implementing」** に自動変更され、ワークフローが実行される
4. ワークフロー完了後、PR が作成され、ステータスが **「pr review」** に遷移する
5. PR の description に `Closes CU-{task_id}` が含まれていることを確認

> **Tip**: 仕様策定が不要な軽微なタスクでは、`ready for spec` を経由せず直接 `ready for code` に移動するショートカットフローも使用できます。

## 6. トラブルシューティング

### ワークフローが起動しない

- **オーケストレーターのログを確認**: Cloud Run のログで `task_detected` / `task_dispatched` イベントが出力されているか確認
- **ステータス名の不一致**: ClickUp リストのステータス名が正しく設定されているか確認。ステータスは小文字化して比較されるため、`Ready for Spec` と `ready for spec` はどちらも有効
- **projects.yaml の設定ミス**: `clickup_list_id`、`github_owner`、`github_repo` が正しいか確認
- **GitHub 認証エラー**: オーケストレーターの `GITHUB_PAT` または `GITHUB_APP_*` の権限に、対象リポジトリの Actions トリガー権限（`workflow` スコープ）が含まれているか確認

### `status_validation_failed`

オーケストレーター起動時に ClickUp リストの必要なステータスが見つからない場合に発生します。ClickUp リストに全ステータスが設定されているか、タイポがないか確認してください。

### `permission_denied` / 権限エラー

- **GitHub Secrets**: `CLICKUP_API_TOKEN` と `CLAUDE_CODE_OAUTH_TOKEN` が正しく設定されているか確認
- **GitHub Token のスコープ**: ワークフローの `permissions` に `contents: write` と `pull-requests: write` が設定されていることを確認（agent.yaml にデフォルトで含まれています）
- **ClickUp API Token**: トークンが有効であり、対象リストへのアクセス権があるか確認

### Bot rejected / Claude Code Action エラー

- `CLAUDE_CODE_OAUTH_TOKEN` が正しく設定されているか確認
- `claude /install-github-app` で対象リポジトリに GitHub App がインストールされているか確認
- ワークフロー内の `allowed_bots: '*'` により、すべての Bot トリガーが許可されています。この設定を変更していないか確認

### ワークフローがタイムアウトする

agent.yaml の `timeout-minutes` は **30 分**に設定されています。大規模なリポジトリや複雑なタスクでタイムアウトする場合は、タスクの粒度を小さくすることを検討してください。
