# 処理フロー図

## 1. 全体ステータスフロー

タスクが `idea draft` から `closed` に至るまでのステータス遷移。人間の操作と自動処理が交互に行われる。

```mermaid
stateDiagram-v2
    [*] --> idea_draft : タスク作成

    idea_draft --> ready_for_spec : 人間が手動で移動

    ready_for_spec --> generating_spec : Orchestrator が検知\nステータス更新 + workflow_dispatch
    generating_spec --> spec_review : Claude Code 成功
    generating_spec --> ready_for_spec : Claude Code 失敗\n(エラー時リセット)

    spec_review --> ready_for_code : 人間がレビュー後に移動

    ready_for_code --> implementing : Orchestrator が検知\nステータス更新 + workflow_dispatch
    implementing --> pr_review : Claude Code 成功
    implementing --> ready_for_code : Claude Code 失敗\n(エラー時リセット)

    pr_review --> closed : 人間がレビュー＆マージ後に移動
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

10秒間隔（デフォルト）で実行される1ティックの処理内容。リコンシリエーションによる状態修正後にタスクの検知・ディスパッチを行う。

```mermaid
flowchart TD
    START([tick 開始]) --> RECONCILE[reconcile: 実行中タスクの状態確認]

    RECONCILE --> FOR_RUNNING{実行中タスクあり?}
    FOR_RUNNING -- Yes --> GET_TASK[ClickUp API: タスク取得]
    GET_TASK --> GET_ERR{取得エラー?}
    GET_ERR -- Yes --> SKIP[スキップ\n次のタスクへ]
    GET_ERR -- No --> IS_TERMINAL{終端ステータス?\nclosed}
    IS_TERMINAL -- Yes --> RELEASE_TERMINAL[state.Release\nクレーム解除]
    IS_TERMINAL -- No --> IS_PROCESSING{処理中ステータス?\ngenerating spec / implementing}
    IS_PROCESSING -- Yes --> CONTINUE[何もしない\n次のタスクへ]
    IS_PROCESSING -- No --> RELEASE_RECONCILE[state.Release\n不整合のため解除]

    RELEASE_TERMINAL --> FOR_RUNNING
    SKIP --> FOR_RUNNING
    CONTINUE --> FOR_RUNNING
    RELEASE_RECONCILE --> FOR_RUNNING

    FOR_RUNNING -- No --> FETCH[ClickUp API: タスク一覧取得]
    FETCH --> FETCH_ERR{取得エラー?}
    FETCH_ERR -- Yes --> END_ERR([tick 終了\nエラーログ])
    FETCH_ERR -- No --> FOR_TASKS{トリガー対象タスクあり?\nready for spec / ready for code}

    FOR_TASKS -- Yes --> DISPATCH[dispatch: タスクディスパッチ]
    DISPATCH --> CLAIM{state.Claim 成功?}
    CLAIM -- No\n既にクレーム済み --> WARN[警告ログ\nスキップ]
    CLAIM -- Yes --> PHASE[PhaseFromStatus\nSPEC or CODE 判定]
    PHASE --> UPDATE_STATUS[ClickUp API: ステータス更新\ngenerating spec / implementing]
    UPDATE_STATUS --> UPDATE_ERR{更新エラー?}
    UPDATE_ERR -- Yes --> RELEASE_U[state.Release] --> RETRY_U[scheduleRetry attempt=1]
    UPDATE_ERR -- No --> TRIGGER[GitHub Actions: workflow_dispatch]
    TRIGGER --> TRIGGER_ERR{トリガーエラー?}
    TRIGGER_ERR -- Yes --> RELEASE_T[state.Release] --> RETRY_T[scheduleRetry attempt=1]
    TRIGGER_ERR -- No --> MARK[state.MarkRunning\nタスクを実行中に登録]

    WARN --> FOR_TASKS
    MARK --> FOR_TASKS
    RETRY_U --> FOR_TASKS
    RETRY_T --> FOR_TASKS

    FOR_TASKS -- No --> END([tick 終了])
```

---

## 3. リトライフロー

ClickUp API 更新または GitHub Actions トリガーが失敗した場合に実行される指数バックオフリトライ。最大遅延は 300,000 ms（5分）。

```mermaid
flowchart TD
    FAIL([ディスパッチ失敗]) --> SCHEDULE["scheduleRetry(taskID, phase, attempt)\n遅延 = min(10000 × 2^(attempt-1), 300000) ms"]
    SCHEDULE --> EXISTING{既存タイマーあり?}
    EXISTING -- Yes --> STOP_OLD[既存タイマーをキャンセル]
    EXISTING -- No --> SET_TIMER
    STOP_OLD --> SET_TIMER[time.AfterFunc で遅延後に handleRetry をスケジュール]

    SET_TIMER --> WAIT([遅延待機])
    WAIT --> HANDLE[handleRetry 実行]
    HANDLE --> GET_TASK2[ClickUp API: タスク取得]
    GET_TASK2 --> GET_ERR2{取得エラー?}
    GET_ERR2 -- Yes --> RETRY_AGAIN["scheduleRetry(taskID, phase, attempt+1)"]
    RETRY_AGAIN --> SCHEDULE

    GET_ERR2 -- No --> IS_TRIGGER{トリガーステータス?\nready for spec / ready for code}
    IS_TRIGGER -- Yes --> DISPATCH2[dispatch: タスクディスパッチ再実行]
    IS_TRIGGER -- No --> RELEASE2[state.Release\nリトライ中止]

    DISPATCH2 --> SUCCESS{成功?}
    SUCCESS -- Yes --> DONE([完了])
    SUCCESS -- No --> SCHEDULE

    RELEASE2 --> ABORT([リトライ中止\nステータスが変わった])

    subgraph 遅延計算例
        D1["attempt=1: 10,000 ms (10秒)"]
        D2["attempt=2: 20,000 ms (20秒)"]
        D3["attempt=3: 40,000 ms (40秒)"]
        D4["attempt=5: 160,000 ms (2分40秒)"]
        D5["attempt=6以降: 300,000 ms (5分) ← 上限"]
    end
```
