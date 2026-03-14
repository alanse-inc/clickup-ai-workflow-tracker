# Development Guide

本プロジェクト（clickup-ai-workflow-tracker）自体の開発フローを定義する。
Claude Code を活用した AI 駆動開発を前提とし、人間は問題定義・レビュー・検証に集中する。

## 1. Development Workflow

### 1.1 Overview

```
GitHub Issue → 仕様策定 → 仕様レビュー → 実装 → テスト → PR → コードレビュー → マージ
     人間       Claude      人間        Claude   Claude  Claude    人間       人間
```

### 1.2 Phase Detail

#### Phase 1: Issue 作成（人間）

GitHub Issues にタスクを作成する。

記載する内容:
- **背景**: なぜこの変更が必要か。
- **ゴール**: 何が達成されれば完了か。
- **制約**: 技術的制約、互換性要件等。
- **参考情報**: 関連する Issue、ドキュメント、コード箇所。

Context Packing の原則に基づき、脳内の関連情報をすべて Issue に書き出す。
曖昧な指示は AI の出力品質を下げるため、「何を作るか」ではなく「なぜ作るか」に注力する。

#### Phase 2: 仕様策定（Claude Code）

Claude Code で Issue の内容をもとに仕様を策定する。

```bash
claude "GitHub Issue #<number> の内容を読んで、実装仕様を作成してください"
```

仕様に含める内容:
- 変更対象ファイルと変更概要。
- 新規追加する型・関数・パッケージのインターフェース設計。
- エッジケースとエラーハンドリング方針。
- テスト方針。

仕様は Issue のコメントとして記録する。

#### Phase 3: 仕様レビュー（人間）

Claude が作成した仕様を人間がレビューする。

確認観点:
- 要件を正しく理解しているか。
- 設計が過剰に複雑でないか。
- SPEC.md のアーキテクチャと整合しているか。
- テスト方針が妥当か。

問題があれば Issue にフィードバックコメントを追加し、Phase 2 に戻る。

#### Phase 4: 実装（Claude Code）

仕様確定後、Claude Code で実装する。

```bash
claude "GitHub Issue #<number> の仕様に基づいて実装してください"
```

#### Phase 5: PR・コードレビュー（人間 + Claude Code）

Claude Code で PR を作成する。人間がコードレビューを行い、問題があれば修正を依頼する。

## 2. Session Design

### 2.1 1 Session 1 Task

1つの Claude Code セッションでは 1つの Issue のみを扱う。
複数タスクを1セッションに詰め込むと Context Rot（文脈劣化）が発生し、出力品質が低下する。

### 2.2 Session Lifecycle

```
1. Issue の内容を Claude Code に伝える
2. 必要に応じてコードベースの調査を依頼する
3. 実装を依頼する
4. テストの実行を確認する
5. コミット・PR 作成を依頼する
6. セッション終了
```

セッションが長くなりすぎた場合（大きな機能追加等）は、タスクを分割して新しいセッションで続行する。

### 2.3 Subagent の活用

調査や探索的なタスクは Subagent に委譲し、メインセッションの Context を汚染しない。

- コードベースの構造調査
- 外部 API の仕様確認
- 既存実装パターンの把握

## 3. Quality Assurance

### 3.1 TDD (Test-Driven Development)

テストは AI 駆動開発における最大の品質保証手段である。

開発サイクル:
1. **Red**: まずテストを書く（または書かせる）。テストが失敗することを確認。
2. **Green**: テストが通る最小限の実装を行う。
3. **Refactor**: テストが通る状態を維持しつつリファクタリング。

Claude Code への指示例:
```
まず <機能名> のテストを書いてください。その後、テストが通る実装を行ってください。
```

### 3.2 Hooks（自動チェック）

Claude Code の PostToolUse Hook で、ファイル変更時に自動的に lint・フォーマット・型チェックを実行する。

`.claude/settings.json` に設定:
`.claude/settings.json` を参照。Hook は Go ファイル変更時に `go fmt` と `go vet` を自動実行する。

これにより、Claude Code がコードを変更するたびに自動でフォーマットと静的解析が走り、問題を即座に検知できる。

### 3.3 Linter / Static Analysis

CI で以下を実行する:

- `go fmt ./...`: フォーマット。
- `go vet ./...`: 静的解析。
- `go test ./...`: テスト。

## 4. CLAUDE.md Design

プロジェクトルートの `CLAUDE.md` に、Claude Code へのプロジェクト固有の指示を記載する。

設計原則:
- **60〜150行**に収める。長すぎると Context を浪費する。
- **Progressive Disclosure**: 概要はルートに、詳細はサブディレクトリの `CLAUDE.md` に分離。
- **事実のみ記載**: 「こうしてほしい」ではなく「こうなっている」を中心に。

含めるべき内容:
- プロジェクト概要と SPEC.md への参照。
- ビルド・テスト・実行のコマンド。
- ディレクトリ構成と各パッケージの責務。
- コーディング規約（Go の慣習に従う旨等）。
- 環境変数の一覧。

## 5. Branch Strategy

| ブランチ | 用途 |
|---------|------|
| `main` | 本番相当。常にデプロイ可能な状態を維持。 |
| `feature/<issue-number>-<short-desc>` | 機能開発。Issue 番号を含める。 |
| `fix/<issue-number>-<short-desc>` | バグ修正。 |

PR マージは Squash Merge を基本とする。

## 6. Commit Convention

```
<type>: <summary>

<body (optional)>

Refs #<issue-number>
```

type:
- `feat`: 新機能
- `fix`: バグ修正
- `refactor`: リファクタリング
- `test`: テスト追加・修正
- `docs`: ドキュメント
- `chore`: CI、依存関係等

## 7. Development Checklist

新しい Issue に取り組む際のチェックリスト:

- [ ] Issue に背景・ゴール・制約が記載されている
- [ ] 仕様を Claude Code で策定し、Issue にコメントとして記録した
- [ ] 仕様を人間がレビューし、承認した
- [ ] feature ブランチを作成した
- [ ] テストを先に書いた（TDD）
- [ ] 実装が完了し、全テストがパスしている
- [ ] `go fmt` / `go vet` をパスしている
- [ ] PR を作成し、Issue と紐付けた
- [ ] 人間がコードレビューを完了した
