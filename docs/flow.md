# 処理フロー図

## 1. 全体ステータスフロー

タスクが `idea draft` から `closed` に至るまでのステータス遷移。人間の操作と自動処理が交互に行われる。

```mermaid
stateDiagram-v2
    [*] --> idea_draft : タスク作成

    idea_draft --> ready_for_spec : 人間が移動

    ready_for_spec --> generating_spec : Orchestrator が検知・ディスパッチ
    generating_spec --> spec_review : 成功
    generating_spec --> ready_for_spec : 失敗（リセット）

    spec_review --> ready_for_code : 人間が移動

    ready_for_code --> implementing : Orchestrator が検知・ディスパッチ
    implementing --> pr_review : 成功
    implementing --> ready_for_code : 失敗（リセット）

    pr_review --> closed : PR マージ検知で自動遷移\nまたは人間が手動で移動
    closed --> [*]

    state "idea draft" as idea_draft
    state "ready for spec" as ready_for_spec
    state "generating spec" as generating_spec
    state "spec review" as spec_review
    state "ready for code" as ready_for_code
    state "implementing" as implementing
    state "pr review" as pr_review
```

---

## 2. Orchestrator の tick 処理フロー

10秒間隔（デフォルト）で実行される1ティックの処理。reconcile → fetch → dispatch の順で実行する。

### 2.1 tick 全体

```mermaid
flowchart TD
    START([tick 開始]) --> RECONCILE[reconcile\n実行中タスクの状態確認]
    RECONCILE --> FETCH[ClickUp API: タスク一覧取得]
    FETCH --> FETCH_ERR{取得エラー?}
    FETCH_ERR -- Yes --> END_ERR([tick 終了])
    FETCH_ERR -- No --> FOR_TASKS{トリガー対象あり?}
    FOR_TASKS -- Yes --> DISPATCH[dispatch]
    DISPATCH --> FOR_TASKS
    FOR_TASKS -- No --> END([tick 終了])
```

### 2.2 reconcile（実行中タスクごと）

```mermaid
flowchart TD
    GET[ClickUp API: タスク取得] --> GET_ERR{取得エラー?}
    GET_ERR -- Yes --> SKIP[スキップ]
    GET_ERR -- No --> PR{PR Review?\nPRChecker 有効?}
    PR -- "マージ済み" --> CLOSE[closed に自動遷移・解放]
    PR -- "未マージ" --> KEEP[維持]
    PR -- No --> TERMINAL{終端?}
    TERMINAL -- Yes --> RELEASE1[解放]
    TERMINAL -- No --> PROCESSING{処理中?}
    PROCESSING -- Yes --> KEEP
    PROCESSING -- No --> RELEASE2[不整合のため解放]
```

### 2.3 dispatch（タスクごと）

```mermaid
flowchart TD
    CLAIM{Claim 成功?} -- No --> SKIP[スキップ]
    CLAIM -- Yes --> UPDATE[ClickUp: ステータスを処理中に更新]
    UPDATE -- エラー --> RETRY1[解放 → scheduleRetry]
    UPDATE -- OK --> TRIGGER[GitHub Actions: workflow_dispatch]
    TRIGGER -- エラー --> RETRY2[解放 → scheduleRetry]
    TRIGGER -- OK --> RUNNING[MarkRunning]
```

---

## 3. リトライフロー

ディスパッチ失敗時に指数バックオフでリトライする。遅延 = `min(10000 × 2^(attempt-1), 300000)` ms。

```mermaid
flowchart TD
    FAIL([失敗]) --> WAIT["遅延待機\n10s → 20s → 40s → ... → 最大5分"]
    WAIT --> GET[ClickUp API: タスク取得]
    GET -- エラー --> WAIT
    GET -- OK --> TRIGGER{まだトリガーステータス?}
    TRIGGER -- Yes --> DISPATCH[dispatch 再実行]
    TRIGGER -- No --> RELEASE[解放・リトライ中止]
    DISPATCH -- 失敗 --> WAIT
    DISPATCH -- 成功 --> DONE([完了])
```
