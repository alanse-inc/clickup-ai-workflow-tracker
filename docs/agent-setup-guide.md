# agent.yaml セットアップガイド

新しいリポジトリに `agent.yaml` を導入し、ClickUp タスク駆動で Claude Code による仕様作成・コード実装を自動化する手順です。

## クイックチェックリスト

> 全体像を把握するためのチェックリストです。詳細は各セクションを参照してください。

- [ ] ClickUp に GitHub Integration をインストール済み
- [ ] `agent.yaml` を対象リポジトリにコピー
- [ ] GitHub Secrets を設定（`CLICKUP_API_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`）
- [ ] ClickUp リストを作成しステータスを設定
- [ ] `projects.yaml` にエントリを追加してデプロイ
- [ ] 動作確認（SPEC → CODE フロー）

---

## 前提条件

- ClickUp に **GitHub Integration** がインストール済み（Settings > Integrations > GitHub）
- 対象リポジトリへの管理者権限
- `clickup-ai-orchestrator-deployment` リポジトリへのアクセス権

---

## Step 1: agent.yaml のコピー

本リポジトリの `.github/workflows/agent.yaml` を対象リポジトリにそのままコピーします。

```bash
mkdir -p .github/workflows
cp /path/to/clickup-ai-orchestrator/.github/workflows/agent.yaml .github/workflows/agent.yaml
```

ファイルの内容は変更不要です。設定値はオーケストレーターから `workflow_dispatch` の入力パラメータとして渡されます。

---

## Step 2: GitHub Secrets の設定

対象リポジトリの **Settings > Secrets and variables > Actions** で設定します。

### 必須

| Secret 名 | 説明 | 設定方法 |
|-----------|------|---------|
| `CLICKUP_API_TOKEN` | ClickUp API トークン | ClickUp > Settings > Apps > API Token で発行 |
| `CLAUDE_CODE_OAUTH_TOKEN` | Claude Code Action 用トークン | ターミナルで `claude /install-github-app` を実行（自動登録される） |

> API トークンはユーザー単位で発行されます。Bot 用サービスアカウントでの発行を推奨します。

### 推奨: CI 自動トリガー用

Claude Code が作成した PR で CI を自動トリガーするには、以下のいずれかを設定します。

**方法 A: GitHub App（推奨）**

| Secret 名 | 説明 |
|-----------|------|
| `CI_APP_ID` | オーケストレーターと同じ GitHub App の App ID |
| `CI_APP_PRIVATE_KEY` | 同 App の Private Key |

Organization Secret にする場合は `MYORG_CI_APP_ID` のように組織名を含め、`agent.yaml` の `app-id` / `private-key` を差し替えてください。

**方法 B: Personal Access Token（代替）**

| Secret 名 | 必要な権限（Fine-grained） |
|-----------|--------------------------|
| `GITHUB_PAT` | Contents: Read and write / Pull requests: Read and write |

> トークンの優先順位: **GitHub App > GITHUB_PAT > GITHUB_TOKEN**。いずれも未設定の場合は `GITHUB_TOKEN` にフォールバックしますが、CI は自動トリガーされません。

### オプション

| Secret 名 | 説明 |
|-----------|------|
| `CLICKUP_AGENT_ERROR_FIELD_ID` | ワークフロー失敗時のエラーメッセージを ClickUp タスクに記録するカスタムフィールド ID |

<details>
<summary>CLICKUP_AGENT_ERROR_FIELD_ID の取得手順</summary>

1. ClickUp の対象リストに **`agent_error`**（テキスト型）カスタムフィールドを作成
2. API でフィールド ID を取得:
   ```bash
   curl -s "https://api.clickup.com/api/v2/list/{list_id}/field" \
     -H "Authorization: <CLICKUP_API_TOKEN>" \
     | jq '.fields[] | select(.name == "agent_error") | .id'
   ```
3. 取得した ID を GitHub Secrets に登録
</details>

---

## Step 3: オーケストレーター側の設定

### 3-1. ClickUp リストの作成

対象プロジェクト用のリストを作成し、以下のステータスを **この順番で** 設定します。

| ステータス名 | 担当 | 説明 |
|-------------|------|------|
| `ready for spec` | 人間 | AI に仕様作成を依頼（トリガー） |
| `generating spec` | AI | 仕様書を生成中 |
| `spec review` | 人間 | 仕様書をレビュー |
| `ready for code` | 人間 | AI に実装を依頼（トリガー） |
| `implementing` | AI | コード実装・PR 作成中 |
| `pr review` | 人間 | PR をレビュー・マージ |
| `closed` | — | 完了（Closed カテゴリ） |

> ステータス名は小文字で比較されるため、`Ready for Spec` でも `ready for spec` でも動作します。

既存リストのステータス名を変更したくない場合は、オーケストレーターの環境変数で既存のステータス名に合わせることができます。詳細は [README.md](../README.md) の「ClickUp ステータス名のカスタマイズ」を参照してください。

### 3-2. projects.yaml にエントリを追加

`clickup-ai-orchestrator-deployment` リポジトリの `projects.yaml` に追加します。

```yaml
projects:
  - clickup_list_id: "XXXXXXXXX"       # ClickUp リスト ID（URL の /li/{list_id} から取得）
    github_owner: "your-org"
    github_repo: "your-repo"
    # github_workflow_file: "agent.yaml"  # optional (default: agent.yaml)
```

### 3-3. デプロイ

`projects.yaml` の変更を PR 経由でマージ後、deployment リポジトリの GitHub Actions で **Deploy to Cloud Run** ワークフローを手動実行（tag=main）してください。

---

## Step 4: 動作確認

### SPEC フェーズ

1. ClickUp でタスクを作成し、タスク名と説明を記述
2. ステータスを **「ready for spec」** に移動
3. 約 10 秒で **「generating spec」** に自動変更される
4. Actions タブで「AI Agent」ワークフローの実行を確認
5. 完了後、タスクの Description に仕様書が書き込まれ **「spec review」** に遷移

### CODE フェーズ

1. 仕様書を確認・必要に応じて修正
2. ステータスを **「ready for code」** に移動
3. **「implementing」** に自動変更、ワークフロー実行
4. 完了後、PR が作成され **「pr review」** に遷移
5. PR description に `Closes CU-{task_id}` が含まれていることを確認

> 仕様策定が不要なタスクは `ready for spec` を経由せず直接 `ready for code` に移動できます。

---

## トラブルシューティング

| 症状 | 確認ポイント |
|------|------------|
| ワークフローが起動しない | Cloud Run ログで `task_detected` / `task_dispatched` が出ているか確認。`projects.yaml` の設定値やステータス名のタイポをチェック。オーケストレーターの GitHub 認証に `workflow` スコープがあるか確認 |
| `status_validation_failed` | ClickUp リストに全ステータスが設定されているか確認（タイポ注意） |
| `permission_denied` | `CLICKUP_API_TOKEN` と `CLAUDE_CODE_OAUTH_TOKEN` が正しいか確認。ワークフローの `permissions` に `contents: write` と `pull-requests: write` があるか確認 |
| Bot rejected / Claude Code Action エラー | `claude /install-github-app` で対象リポジトリに App がインストールされているか確認。`allowed_bots: '*'` が変更されていないか確認 |
| タイムアウト | `timeout-minutes` は 30 分。タスクの粒度を小さくすることを検討 |
