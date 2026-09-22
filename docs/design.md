# design

## 目標

git/contrib/diff-highlight(Perl)を Go で完全再現する。
unified diff を受け取り、`-`/`+` ペアの行内差分だけを ANSI 強調して
出力するストリーミングフィルタ。入出力はオラクルと同じ
(「時折ハイライトが入る」以外は入力をそのまま通す)。

## 方針

- 依存は最小限。標準ライブラリを基本とするが、Windows 対応などに
  必要なら最小限の外部依存(例: `golang.org/x/sys`)は許容する。
- Windows でも動作すること(Linux/macOS 専用のコードにしない)。
- Go 1.27 前提(go.mod の `go` ディレクティブも 1.27)。
- `go fix` / `gofmt` / `go vet` が常に通る状態を保つ。
- `package main` をトップレベルに置き、
  `go install github.com/podhmo/diff-highlight@latest` でインストールできる
  構成にする(既存の空の `main.go` の位置を維持)。内部実装は必要に応じて
  `internal/` パッケージに分けてよいが、外部に公開する API は作らない。

## 構成

- `main.go`: stdin を 1 行ずつ読み、状態機械に流し `bufio.Writer` で stdout へ。
- コアロジックを別ファイル/`internal/` に分ける場合も、関数名・責務は
  オラクルの `DiffHighlight.pm`(`handle_line` / `flush` / `show_hunk` /
  `highlight_pair` / `highlight_line` / `color_config`)に対応させると
  移植の追跡が容易。
- 色設定は実行時に `git config --type=color --get-regexp '^color\.diff-highlight\.'`
  を読む。git 不在・失敗時もデフォルトで動作させる。

## テスト

- オラクルの `t9400-diff-highlight.sh` の観点を golden テストとして移植する。
  詳細は `docs/tasks.md`。
