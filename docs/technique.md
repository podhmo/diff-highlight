# diff-highlight 技術メモ

オラクル: https://github.com/git/git/tree/master/contrib/diff-highlight
(本リポジトリの `oracle/` 以下に当該ソースをスナップショットとして保存している)

## 目的と全体像

unified diff(`git log -p --color` など)を後処理するストリーミングフィルタ。
「削除行 `-` と追加行 `+` のペア」を見つけ、行内で実際に変わった部分だけを
ANSI カラーで強調する。行指向 diff を壊さないよう、入力と出力は
「時折ハイライトが入る」以外は完全に同じになることを目指す。

構成(git 本家側):

- `diff-highlight.perl`: 8 行の薄いラッパー。`SIG{PIPE} = 'DEFAULT'` を設定し
  `DiffHighlight::highlight_stdin()` を呼ぶだけ。
- `DiffHighlight.pm`: 実装本体(約 300 行)。
- `Makefile`: `shebang + DiffHighlight.pm + diff-highlight.perl` を `cat` して
  単一実行ファイル `diff-highlight` を生成する。
- `t/t9400-diff-highlight.sh`: git のテストフレームワーク(test-lib.sh)上の
  シェルテスト。

Go 版では cat 結合や Makefile は不要。`package main` 1 つでよい。

## 状態と I/O モデル

フィルタの内部状態:

- `@removed` / `@added`: 現在のハンク内でバッファした `-` 行 / `+` 行。
- `$in_hunk`: 直前の行がハンク内にいたか。
- `$graph_indent`: `git log --graph` の左側グラフ描画("| * " 等)の可視幅。
  この幅だけ先頭を捨ててから diff 判定する。

出力はコールバック経由:

- `$line_cb`: 行の出力(デフォルト `print`)。複数行まとめて呼ばれることもある。
- `$flush_cb`: 「論理的な塊が終わった」ときの出力フラッシュ(デフォルト stdout flush)。

Go 版ではこれらは `bufio.Writer` + `Flush()` に置き換えられる。

## 正規表現(アトム)

- `COLOR = /\x1b\[[0-9;]*m/`: SGR シーケンス 1 個。
- `BORING = /$COLOR|\s/`: カラーまたは空白。
- `RESET = "\x1b[m"`。

## handle_line(行ごとの状態機械)

1. **グラフ行の検出**:
   `/^(?:COLOR?\|COLOR? )* COLOR?\*COLOR? (?:COLOR?\|COLOR? )* * /x` に
   マッチしたら `--graph` のコミット開始行とみなす。
   - まず `flush()`(前のハンクのインデントと混ざらないよう先に吐き出す)。
   - `$graph_indent = visible_width($&)`(マッチした prefix の可視幅)。

2. **グラフインデント処理**: `$graph_indent > 0` のとき、
   - 行の長さが indent 未満なら `$graph_indent = 0` に戻す。
   - そうでなければ `visible_substr($_, $graph_indent)` で先頭を可視幅分だけ
     捨てたものを判定対象にする(出力には `$_` ではなくオリジナル `$orig` を使う点に注意)。

3. **ハンク状態機械**:
   - `!$in_hunk` なら: そのまま `$line_cb` で出力し、
     行が `/^COLOR*\@\@ /`(`@@ ` で始まる、色付き可)なら `$in_hunk = 1`。
     - `@@@`(combined diff)は `@@ ` にマッチしないため**そのまま素通し**
       される(テスト "ignores combined diffs")。
   - `$in_hunk` で `/^COLOR*-/` なら `@removed` に push。
   - `$in_hunk` で `/^COLOR*\+/` なら `@added` に push。
   - それ以外(コンテクスト行、`\ No newline at end of file` 等):
     `flush()` してから出力し、`$in_hunk = /^COLOR*[\@ ]/` で再評価。
     (ハンク中の `\` 行はここでハンクを終了させる。)

4. **ブランク行で flush_cb**: `/^$/` なら `$flush_cb->()`。
   `git log` の出力がコミット間で空白行を挟むヒューリスティックで、
   長時間無出力になりがちな `git log -S` でも早期に見えるようにするため。

## show_hunk / flush

`flush()` は `show_hunk(\@removed, \@added)` して両バッファを空にする。
`highlight_stdin()` は入力終了後にも `flush()` を呼ぶ(末尾ハンク対策)。

`show_hunk($a, $b)`:

- 片側が空なら比較不能 → そのまま順に出力。
- 行数が違えば単純にそのまま出力(位置合わせを諦める。\
  "simple and stupid" 方針。テスト "mismatched hunk size" は
  現状 test_expect_failure)。
- 行数が同じなら **i 番目の削除行と i 番目の追加行**を `highlight_pair` に
  渡し、削除行は即出力、追加行はキューにためて最後にまとめて出力
  (削除ブロック → 追加ブロックの順序を保つため)。

## highlight_pair(コアアルゴリズム)

2 行をトークン列に分解(`split_line`)してから処理する。

### split_line

1. 行を `/(COLOR+)/` で分割(キャプチャ付きなので区切りも残る)。
2. 各要素が COLOR ならそのまま 1 トークン、そうでなければ `split //`
   で **1 文字 1 トークン** に分解。
3. その前に `utf8::decode` しておくことで、マルチバイト UTF-8 も
   「1 文字」として扱う(分割後に `utf8::encode` でバイト列に戻す)。

Go 版では「ANSI シーケンスをアトム、残りは rune 単位」で等価。
結合文字(combining code points)は Perl 側でも 1 文字として扱えず
テストが test_expect_failure になっている → Go 版も素の rune 分割で
同じ限界を受け入れるのが移植として素直。

### 共通 prefix スキャン

`$pa`, `$pb` を先頭から進める。各ステップで:

- `a[$pa]` が COLOR なら `$pa++`(色を飛ばす)。
- `b[$pb]` が COLOR なら `$pb++`。
- 両者が等しいトークンなら両方進める。
- **未だ `seen_plusminus` が立っておらず** `a[$pa] eq '-'` かつ
  `b[$pb] eq '+'` なら、先頭の diff マーカー対として両方進めて
  `seen_plusminus = 1`(行頭の `-`/`+` は違って当然なので一致扱い)。
- それ以外で打ち切り。

### 共通 suffix スキャン

`$sa = $#a`, `$sb = $#b` から末尾向きに。COLOR を飛ばしつつ
等しいトークンが続く間だけデクリメント。prefix/suffix が
交差する範囲(`$sa >= $pa` かつ `$sb >= $pb`)まで。

### is_pair_interesting(全体ハイライト抑止)

「行全体が強調されるなら、もはや行単位 diff と同じで意味がない」
のでハイライトしないルール。

- `$pa == @$a` or `$pb == @$b`(= 2 行が完全一致、prefix が行全体を消費)
  → not interesting。`foo` → `foo`(末尾改行の有無差)のような
  diff が non-minimal なときに起きる。
- prefix/suffix が「つまらない」ものだけなら not interesting:
  - `visible_substr($prefix_a, $graph_indent)` が
    `/^COLOR*-$BORING*$/`(= `-` と空白/色だけ)なら prefix_a は boring。
    `+` 側も同様。
  - suffix_a/suffix_b が `/^BORING*$/` なら boring。
  - 4 つとも boring → not interesting(ハイライト範囲が行全体に
    なってしまうケース)。

### highlight_line(色の付け方 2 モード)

トークン列を `[0..prefix-1] | [prefix..suffix] | [suffix+1..end]` に
3 分割して join した `start | mid | end` に対し:

- **normal 色ありモード**(`theme[0]` が定義):
  既存の色を全部剥がし(`s/COLOR//g`)、
  `normal + start + RESET + highlight + mid + RESET + normal + end + RESET + \n`
  を組み立てる(行全体を取り仕切る)。
- **highlight/reset モード**(`theme[0]` 未定義):
  既存色は残したまま、mid の前後にだけ
  `start + highlight + mid + reset + end` を差し込む。

## 色設定(color_config / load_color_config)

- テーマ: `@OLD_HIGHLIGHT` / `@NEW_HIGHLIGHT`、各 3 要素
  `(normal, highlight, reset)`。
- 外部(モジュール利用側)から設定されていなければ遅延で
  `git config --type=color --get-regexp '^color\.diff-highlight\.'`
  を実行して読む(stderr は devnull へ)。**git が実行できなくても動く**
  よう自前のフォールバックを持つ設計(コメント参照)。
- デフォルト: `oldhighlight = "\x1b[7m"`(反転), `oldreset = "\x1b[27m"`。
  `new*` が未指定なら `old*` にフォールバック。
- 設定キー: `color.diff-highlight.{old,new}{Normal,Highlight,Reset}`。

Go 版でも `exec.Command("git", "config", ...)` + フォールバックで
同等にできる。git 非依存にしたければ環境変数や自前パーサも検討(タスク参照)。

## ユーティリティ

- `visible_width($s)`: COLOR トークンを飛ばして可視文字数を数える。
- `visible_substr($s, $n)`: 先頭から可視文字 n 個を捨てた残りを返す
  (COLOR は幅に数えず、そのまま残る)。

## 既知の限界(オラクル README の "Bugs")

1. 1 行に複数の変更があると 1 つのブロブにまとめて強調される
   (`foo(buf, size)` → `foo(obj->buf, obj->size)` は括弧内全体)。
   word-diff 的な境界を入れない限り避けられない設計判断。
2. 複数行ペアは位置(index)でしか対応づけないため、
   上端削除+下端追加のようなケースで誤ペアが起きうる。

## オラクルのテスト観点(t9400-diff-highlight.sh)

`dh_test a b` は「a の内容でコミット → b に書き換えて diff と commit を生成 →
両方を diff-highlight に通し、`@@` 以降を色デコードして期待値と比較」する。

- 先頭/末尾/中間のハイライト
- 行全体が違う場合はハイライトしない
- ハンクの行数不一致(failure 扱い)
- マルチバイト UTF-8 / 結合文字(後者は failure 扱い)
- `--graph`(色なし/色あり、先頭 `-` 付きグラフ)
- combined diff は無視
- 末尾改行の削除(`\ No newline at end of file`)
- 色設定: set/reset モード、normal/highlight モード
