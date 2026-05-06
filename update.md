# erdm 更新方針

ERD の可読性を上げるための、`erdm` プロジェクトの中期改修方針メモ。
背景分析 (末尾「参考: 当初の検討メモ」) を踏まえ、対話で決定した方針を記録する。

---

## 1. 決定事項サマリー

| 区分 | 決定内容 |
| --- | --- |
| 改修スコープ | **ELK / Web UI 移行**。Graphviz/DOT バックエンドは当面並行維持し、新デフォルトを ELK + Web UI にする |
| DOT 既定 `rankdir` | `LR` (左→右)。`--rankdir` 等で切替可能にするかは実装時に判断 |
| エッジ方向 | 図上は **親テーブル → 子テーブル** に正規化 (DB 上の FK の向きとは独立) |
| エッジルーティング | `splines=ortho` (直交ルーティング) |
| グルーピング | `.erdm` の DSL を拡張し、テーブル属性として `@groups[...]` を追加 |
| 複数グループ | 1テーブル複数 group OK。**配列の先頭を primary** とし、cluster 描画は primary のみ採用。残りはフィルタ・色付け等に利用 |
| トップレベル `group` ブロック | **入れない**。`@groups[...]` のみサポート |
| Web UI スタック | React + React Flow + elkjs |
| Web UI 機能 | (a) ERD 閲覧 (ズーム/パン)、(b) `.erdm` スキーマ編集、(c) DDL / SVG / PNG エクスポート |
| 配布形態 | `erdm serve` のようなローカル HTTP サーバーとして起動し、ブラウザで開く |
| `.erdm` 保存 | 起動時に指定された元ファイルを **直接書き換え** |
| 手動配置の座標保存 | スコープに含める。`.erdm` とは **別ファイル** (例: `<name>.erdm.layout.json`) に保存 |
| フロントエンドのバンドル | Vite ビルド成果物を Go の `embed.FS` で同梱し、単一バイナリで配布 |

---

## 2. DSL 拡張仕様 (`@groups[...]`)

### 2.1 構文

```
table user_orders @groups["Order", "User", "Billing"] {
  id          int    primary
  user_id     int
  order_id    int
}
```

- 角括弧 `[...]` 内にカンマ区切りで group 名 (文字列) を並べる。
- 配列の **先頭** が primary group。
- group 名は任意の文字列。`.erdm` 内で初出の group 名がそのまま登場順で扱われる。
- `@groups` を持たないテーブルは "ungrouped" として扱う (cluster なしで描画)。

### 2.2 描画ルール

| 出力 | primary group の扱い | 追加 group の扱い |
| --- | --- | --- |
| Graphviz/DOT | `subgraph cluster_<primary>` に所属 | DOT には現れない (将来は色付けに使う余地あり) |
| ELK / Web UI | groupNode (親ノード) に所属 | バッジ・色帯・フィルタで表示 |

### 2.3 ungrouped の扱い

- いずれの cluster/groupNode にも所属しない。
- ELK 側ではトップレベルに直接配置。

---

## 3. アーキテクチャ概要

```
.erdm
  ↓ (parse: erdm.peg を拡張)
内部スキーマモデル (Go)
  ├─ レガシー: DOT 生成 → Graphviz → SVG/PNG/HTML  (互換維持)
  └─ 新系統:
       ├─ HTTP API:
       │    GET  /api/schema           現在の .erdm を JSON で返す
       │    PUT  /api/schema           .erdm に書き戻し
       │    GET  /api/layout           layout.json を返す
       │    PUT  /api/layout           layout.json を更新
       │    GET  /api/export/{ddl|svg|png}
       └─ 静的アセット (embed.FS):
            React + React Flow + elkjs の SPA
              - ELK で初期レイアウト
              - 手動ドラッグで位置調整 → /api/layout に保存
              - 編集 UI で .erdm を更新 → /api/schema に保存
```

---

## 4. ファイル構成 (想定)

```
erdm/
├── erdm.go                # 既存メインエントリ
├── erdm.peg(.go)          # @groups[...] 構文を追加
├── internal/              # (新規) スキーマモデル / レイアウト / API ハンドラを段階的に切り出し
│   ├── model/             # テーブル・FK・group の Go 構造体
│   ├── parser/            # PEG をラップする層 (テスト容易化)
│   ├── dot/               # 既存 DOT 出力 (rankdir=LR, splines=ortho 等を反映)
│   ├── elk/               # ELK 用 JSON 出力
│   ├── layout/            # layout.json の I/O
│   └── server/            # `erdm serve` の HTTP ハンドラ
├── frontend/              # (新規) Vite + React + TS + React Flow + elkjs
│   ├── src/
│   └── dist/              # ビルド成果物 (embed.FS で同梱)
├── templates/             # 既存テンプレート (HTML 出力など) は当面維持
└── ...
```

---

## 5. 実装フェーズ

### Phase 1: 既存 DOT 出力の改善 (短期・低リスク)

- `rankdir=LR`, `splines=ortho`, `nodesep=0.8`, `ranksep=1.2`, `concentrate=false` を既定化。
- エッジを **親→子** に正規化 (FK の向きとは独立に図上の向きを揃える)。
- 既存テストの期待値更新 (`test/` 配下の既存 ERD スナップショットを再生成)。

### Phase 2: DSL 拡張 — `@groups[...]`

- `erdm.peg` に `@groups[ "..." ( , "..." )* ]` を追加し、`erdm.peg.go` を再生成。
- 内部モデルにグループ情報を保持。
- DOT 出力で primary group を `cluster_<group>` として描画。
- パーサテスト + DOT 出力スナップショットを追加。

### Phase 3: `internal/model` への切り出し

- 現在 `erdm.go` に集約している処理を `internal/model`, `internal/dot` に分離。
- 既存の CLI 動作は完全互換を維持。

### Phase 4: ELK 用 JSON 出力

- `internal/elk` で内部モデル → ELK JSON への変換を実装。
- group は ELK の階層 (`children` を持つ親ノード) として表現。primary group のみ階層化。
- CLI から `erdm --format=elk schema.erdm` のようにダンプできる経路を用意 (UI 連携と CLI デバッグ両用)。

### Phase 5: `erdm serve` (Web UI 基盤)

- `erdm serve <schema.erdm> [--port=8080] [--no-write]` サブコマンドを追加。
- Go の `net/http` + `embed.FS` で SPA を配信。
- API:
  - `GET /api/schema` → 現スキーマの JSON
  - `PUT /api/schema` → 書き戻し (`--no-write` 時は 403)
  - `GET /api/layout` / `PUT /api/layout` → 座標保存
  - `GET /api/export/ddl` / `svg` / `png`
- フロントは閲覧 + ズーム/パンのみまずリリース。

### Phase 6: フロント — レイアウト調整 & 座標保存

- 起動時に `<schema>.erdm.layout.json` を読み込み、存在すればその座標で復元。
- ドラッグで位置調整、保存ボタンで PUT。
- 新規テーブルは ELK の自動配置にフォールバック。

### Phase 7: フロント — `.erdm` 編集機能

- テーブル / カラム / FK / `@groups` の追加・編集・削除 UI。
- 編集結果を `.erdm` テキストへシリアライズして PUT。
- 編集中はローカル下書きを `localStorage` に保持し、誤操作からの復帰を可能にする。

### Phase 8: エクスポートとドキュメント整備

- DDL / SVG / PNG のダウンロード UI。
- README に新フローを追記。Graphviz バックエンドの位置付けを明記。

---

## 6. オープン課題 / 後続検討

- `--focus <table> --depth N` や `--overview` 等のサブセット出力 CLI は今回は **対象外**。Phase 4 以降の追加機能として検討余地あり。
- ELK 移行後の Graphviz バックエンドの将来 (deprecate するか維持か) は実運用後に判断。
- 既存 HTML テンプレート (`templates/`) と新 Web UI の関係 (置き換え/併存) は Phase 5 着手時に再整理。
- Web UI 編集と Git 管理の衝突回避策 (排他制御や差分プレビュー) は Phase 7 で別途検討。

---

## 7. 参考: 当初の検討メモ (要約)

ERD レイアウト改善の方針を比較した当初メモの要点:

- 自動配置だけで常に読みやすい ERD は困難。**自動レイアウトで初期配置 → 人間が意味的グルーピング・微調整** が現実的。
- 小〜中規模なら Graphviz でも `rankdir=LR` / `splines=ortho` / `nodesep` / `ranksep` の調整 + `rank=same` / `cluster` の自動生成で十分改善可能。
- 大規模 ERD は「全部の線をきれいに」より「**分割出力**」のほうが本質的に有効 (全体概要 / ドメイン別 / テーブル中心 / 差分)。
- インタラクティブ UI を作るなら ELK / elkjs + React Flow のほうが Graphviz より拡張性で有利。
  - 初期配置は ELK、ユーザーが微調整、座標を JSON で保存、新規テーブルだけ自動配置、というフローが現実的。
- 結論: **人間が全部の線を引く必要はない。ただし、人間が意味的なグルーピングと優先順位を与えないと読みやすさには限界がある。**

→ 今回はこの結論を踏まえ、ELK + Web UI への移行を選択した (§1)。
