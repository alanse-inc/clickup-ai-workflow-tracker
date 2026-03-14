# Skill: create-pr

現在のブランチの変更内容から PR を作成する。
"PR作成", "create PR", "プルリクエスト作成" と言われた場合に使用する。

## Instructions

1. 現在のブランチと差分を確認する:
   ```bash
   git status
   git diff main...HEAD
   git log main..HEAD --oneline
   ```
2. 未プッシュの変更があればプッシュする:
   ```bash
   git push -u origin <branch-name>
   ```
3. PR を作成する:
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
4. 作成した PR の URL をユーザーに報告する

## Notes

- PR タイトルは70文字以内
- `Closes #<number>` でマージ時に Issue を自動クローズする
- プッシュ前にユーザーの確認を取る
