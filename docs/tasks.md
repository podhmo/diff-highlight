# tasks

Go で git/contrib/diff-highlight を完全再現するためのタスク。
詳細な仕様は `docs/technique.md`、オラクルは `oracle/` を参照。

方針:

- 依存は標準ライブラリのみ。
- `package main` をトップレベルに置き、
  `go install github.com/podhmo/diff-highlight@latest` でインストールできる構成
  にする(既存の空の `main.go` の位置を維持)。内部実装は必要に応じて
  `internal/` パッケージに分けてよいが、外部に公開する API は作らない。

## 実装タスク

- [ ] ANSI 関連ユーティリティ
  - `COLOR` 相当: `\x1b\[[0-9;]*m` の正規表現。
  - `BORING` 相当: COLOR または空白。
  - `visibleWidth(s)`, `visibleSubstr(s, n)`(色シーケンスを幅 0 として扱う)。
- [ ] トークン分割 `splitLine(line) []string`
  - COLOR 正規表現で分割し、色部分は 1 トークン、それ以外は rune 1 つ = 1 トークン。
  - Perl の `utf8::decode` → `split //` → `utf8::encode` は、Go では
    rune 単位の分割で等価になる。
- [ ] `handleLine` 状態機械
  - `--graph` コミット開始行の検出 regex(オラクルの 5 行にまたがる
    `/x` パターンをそのまま移植)と `$graph_indent` 管理。
  - `$in_hunk` 遷移: `@@ ` で開始、`-`/`+` でバッファ、それ以外で flush。
    `@@@`(combined diff)が `@@ ` にマッチしないことを確認。
  - 空行で出力 flush(ストリーミング用ヒューリスティック)。
- [ ] `showHunk`/`flush`
  - 片側空 or 行数不一致 → 素通し。行数一致のみ i 番目同士をペア化。
  - 削除行は即出力、追加行はキューして後出し(順序維持)。
- [ ] `highlightPair`
  - 共通 prefix スキャン(色スキップ + 先頭の `-`/`+` マーカーの
    `seen_plusminus` 対応)。
  - 共通 suffix スキャン。
  - `isPairInteresting`: 完全一致行、prefix/suffix が boring だけの場合は
    ハイライトしない(行全体ハイライト抑止ルール)。
- [ ] `highlightLine`
  - normal/highlight モード(既存色を剥がして 3 区画に再着色)と
    highlight/reset モード(既存色を残し mid の前後だけ差し込み)の 2 系統。
- [ ] 色設定 `colorConfig`/`loadColorConfig`
  - `git config --type=color --get-regexp '^color\.diff-highlight\.'` を
    実行しキャッシュ。失敗/git 不在でもデフォルトで動くこと。
  - デフォルト: `oldhighlight=\x1b[7m`, `oldreset=\x1b[27m`、
    new 系は old 系へフォールバック。
- [ ] `main`
  - stdin を 1 行ずつ読み `handleLine` に流し、EOF で flush。
  - `bufio.Writer` を使い、空行受信時に `Flush()`。
  - SIGPIPE: Go では stdout への SIGPIPE で既定終了するため、
    オラクルの `$SIG{PIPE} = 'DEFAULT'` と同等。パイプ切断時の挙動を確認。

## テストタスク(オラクルを基準にした golden テスト)

`oracle/t/t9400-diff-highlight.sh` の `dh_test` 相当を Go のテストに移植する。
テスト入力は「`@@` 以降のハンク部分」と期待 ANSI 出力を testdata に置くか、
実 git リポジトリで diff を生成する形を検討(後者は git 依存になるので CI
環境を要確認)。少なくとも以下を網羅する:

- [ ] 先頭/末尾/中間のハイライト
- [ ] 行全体が異なる場合はハイライトしない
- [ ] ハンクの行数不一致 → ハイライトしない(オラクルも failure 想定)
- [ ] マルチバイト UTF-8 を 1 文字として扱う
- [ ] 結合コードポイント(オラクルは test_expect_failure: 現状の限界として
      ドキュメント化 or 同じく既知失敗扱いにする)
- [ ] `--graph`(色なし/色あり/先頭 `-` 付きグラフ)
- [ ] combined diff は素通し
- [ ] 末尾改行の削除(`\ No newline at end of file`)
- [ ] 色設定: set/reset モード、normal/highlight モード
      (git config を実際に読む経路とフォールバック両方)

## 仕上げ

- [ ] `go vet` / `go test ./...` が通る
- [ ] README に Go 版の使い方を追記するかは別途検討
- [ ] `oracle/` をリポジトリに残すか判断(参照用に残すなら
      ライセンス表記の確認: git は GPLv2、本リポジトリも GPLv2 なので整合)
