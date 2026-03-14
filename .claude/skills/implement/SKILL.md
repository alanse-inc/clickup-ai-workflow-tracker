# Skill: implement

GitHub Issue の仕様に基づいてコードを実装する。
"実装して", "implement", "コードを書いて" と言われた場合に使用する。

## Instructions

1. 指定された GitHub Issue とそのコメント（仕様）を `gh issue view <number> --comments` で取得する
2. 仕様コメントの内容を把握する。仕様がない場合は、先に `/spec` を実行するよう促す
3. feature ブランチを作成する:
   ```bash
   git checkout -b feature/<issue-number>-<short-desc>
   ```
4. TDD で実装する:
   - まずテストを書く
   - テストが失敗することを確認する (`go test ./...`)
   - テストが通る最小限の実装を行う
   - すべてのテストがパスすることを確認する
5. `go fmt ./...` と `go vet ./...` がパスすることを確認する
6. 変更をコミットする。コミットメッセージは以下の形式:
   ```
   <type>: <summary>

   Refs #<issue-number>
   ```
7. ユーザーに実装内容を報告し、PR 作成の指示を待つ

## Notes

- 1つの Issue に対して1つのブランチ
- 実装は仕様の範囲内に留める。スコープ外の改善は別 Issue にする
- テストがない実装はしない
- エラーハンドリングはシステム境界（外部API呼び出し）にのみ行う
