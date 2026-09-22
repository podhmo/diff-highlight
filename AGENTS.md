# AGENTS.md

git/contrib/diff-highlight(Perl)の Go 移植。オラクルは `oracle/`、
詳細仕様は `docs/technique.md`、設計方針は `docs/design.md`、
やるべきタスクは `docs/tasks.md` を参照。

## 基本方針

- Go 1.27、依存は最小限(標準ライブラリ基本)、Windows も動作対象。
- `package main` はトップレベル(`go install github.com/podhmo/diff-highlight@latest`)。
- 変更は常に `go fix` / `gofmt` / `go vet` / `go test ./...` が通る状態に保つ。

## バグの記録

- 作業中に気づいたバグ・不審な挙動は、**その時のタスクと関係なく**
  `docs/bugs/` 以下に記録する。1 バグ 1 ファイル、再現入力があれば添える。
- オラクル由来の既知の限界と、Go 版で見つかった本物のバグは区別して書く。
