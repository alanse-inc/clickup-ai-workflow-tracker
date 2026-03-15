---
name: apply-issue
description: >
  GitHub Issue を選択し、仕様作成・実装・PR作成までを一貫して実行するスキル。
  "/apply-issue" で issue 一覧から対話的に選択、"/apply-issue <number>" で直接指定が可能。
  "issue対応", "apply-issue", "issueをやって" と言われた場合に使用する。
---

# Skill: apply-issue

GitHub Issue の選択から仕様作成・実装・コミット・PR 作成まで一貫して実行する。

## Usage

- `/apply-issue` - issue 一覧を表示し、ユーザーに対応する issue を選んでもらう
- `/apply-issue <number>` - 指定された issue 番号に直接対応する

## Instructions

### Mode 1: Issue 番号が指定されていない場合

1. open な issue 一覧を取得する:
   ```bash
   gh issue list --state open
   ```
2. 一覧をユーザーに提示し、各 issue の概要と優先度について簡潔にコメントする
3. ユーザーにどの issue に対応するか選択してもらう
4. 選択された issue 番号で Mode 2 に進む

### Mode 2: Issue 番号が指定されている場合

以下のステップを順に実行する。

#### Step 1: 仕様策定 (/spec)

1. `gh issue view <number>` で issue の内容を取得する
2. SPEC.md を読み、システムアーキテクチャを把握する
3. 既存のコードベースを調査し、関連するファイル・パッケージを特定する
4. 実装仕様を作成し、GitHub Issue にコメントとして投稿する:
   ```bash
   gh issue comment <number> --body "<仕様内容>"
   ```
5. 仕様をユーザーに提示し、レビュー・承認を依頼する
6. フィードバックがあれば仕様を修正する。承認されたら次のステップへ進む

#### Step 2: 実装 (/implement)

1. `gh issue view <number> --comments` で仕様コメントを確認する
2. feature ブランチを作成する:
   ```bash
   git checkout -b feature/<issue-number>-<short-desc>
   ```
3. TDD で実装する:
   - まずテストを書く
   - テストが失敗することを確認する (`go test ./...`)
   - テストが通る最小限の実装を行う
   - すべてのテストがパスすることを確認する
4. `golangci-lint run ./...` と `go vet ./...` がパスすることを確認する
5. 変更をコミットする:
   ```
   <type>: <summary>

   Refs #<issue-number>
   ```

#### Step 3: PR 作成 (/create-pr)

1. リモートにプッシュする前にユーザーの確認を取る
2. プッシュ後、PR を作成する:
   ```bash
   gh pr create --title "<type>: <summary>" --body "$(cat <<'EOF'
   ## Summary
   <変更内容の要約を箇条書き>

   ## Test Plan
   - [ ] `go test ./...` がパスする
   - [ ] `go vet ./...` がパスする

   Closes #<issue-number>
   EOF
   )"
   ```
3. 作成した PR の URL をユーザーに報告する

## Notes

- 各ステップ間でユーザーの確認を挟む。自動で全ステップを走り抜けない
- 仕様レビューでの承認なしに実装に進まない
- 実装は仕様の範囲内に留める。スコープ外の改善は別 Issue にする
- 1つの Issue に対して1つのブランチ・1つの PR
