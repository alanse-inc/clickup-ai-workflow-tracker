# clickup-ai-workflow-tracker

ClickUp 上のタスクをステータス駆動で検知し、GitHub Actions 経由で Claude Code を起動して仕様作成・コード実装を自律的に実行する Go オーケストレーションサービス。

## Why Go?

alanse では TypeScript を主軸に開発していますが、本プロジェクトでは Go を採用しています。

**Goroutine によるシンプルな並行処理**: ポーリングループ、タスクディスパッチ、ステータス監視といった複数の非同期処理を Goroutine と `sync.Mutex` で自然に記述できます。状態管理のためにデータベースや外部キューを持つ必要がなく、インメモリで完結します。

**シングルバイナリ・省リソース**: ビルド成果物は単一バイナリ。安価な VPS に配置して `systemd` でデーモン化するだけで 24 時間稼働します。Node.js ランタイムや依存のインストールが不要で、メモリ消費も極めて少ないです。

**Symphony との対比**: OpenAI Symphony（TypeScript 実装）はローカルマシンで Codex を直接起動する設計ですが、本システムは GitHub Actions にエージェント実行を委譲するため、オーケストレータ自体は軽量なスケジューラに徹します。Go のシンプルさと低フットプリントがこの役割に適しています。

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
