# craft-auto-routing

Craft docs の未分類ドキュメント (Unsorted) を、既存のフォルダ構成を読み取った上で
[TypeSafe](https://docs.typesafe.ai) の `jev` モデルに判定させ、最も適したフォルダへ
自動で振り分ける CLI ツール。

## How it works

1. Craft Connect API (`GET /folders`) から既存のフォルダツリーを取得し、フォルダパス
   (`Projects/Work` のようなブレッドクラム) にフラット化する。
2. 各フォルダに実際に入っているドキュメントのタイトルをいくつかサンプリングし
   (`GET /documents?folderId=...`)、「このフォルダには何が入っているか」を表す説明文を
   フォルダごとに組み立てる。これが既存の構成を読み取る部分。
3. `location=unsorted` のドキュメント一覧を取得し、各ドキュメントの本文
   (`GET /blocks`) とタイトルを TypeSafe SystemOne API の `choice` 質問の `state` として渡す。
   選択肢 (`criteria`) は手順2で作ったフォルダごとの説明文。
4. `jev-latest` が返した `choice` (フォルダID) と `confidence` を見て、閾値以上なら
   `PUT /documents/move` でそのフォルダに移動する。閾値未満は移動せずスキップして報告する。

## Setup

```sh
go build ./cmd/craft-auto-routing
```

環境変数:

| 変数 | 必須 | 説明 |
| --- | --- | --- |
| `CRAFT_API_TOKEN` | ✅ | Craft Connect API の Bearer トークン |
| `CRAFT_BASE_URL` | ✅ | 自分の Craft Connect API のベース URL (例: `https://connect.craft.do/links/<your-link-id>/api/v1`)。Craft の Connect API 設定画面から確認できる |
| `TYPESAFE_API_KEY` | ✅ | TypeSafe の API キー |
| `TYPESAFE_MODEL` | - | デフォルトは `jev-latest` |

トークンは `.env` などに置かず、シェルのシークレット管理 (`direnv`, keychain 等) から
注入すること。

## Usage

```sh
# 既存フォルダ構成とサンプルドキュメントを確認する
craft-auto-routing folders

# まずは dry-run で振り分け計画だけ確認する
craft-auto-routing route --dry-run

# 実際に移動する (confidence 0.7 未満はスキップ)
craft-auto-routing route --min-confidence 0.7

# 特定フォルダに溜まったドキュメントを丸ごと読み直して再振り分けする
craft-auto-routing route --source-folder "Projects/Inbox" --dry-run
```

主なフラグ (`route`):

- `--dry-run`: 分類結果を表示するだけで、実際には移動しない。
- `--location` (default `unsorted`): ドキュメントの取得元とする Craft の location (`unsorted`, `trash`, `templates`, `daily_notes`)。`--source-folder` とは併用不可。
- `--source-folder`: `--location` の代わりに、既存の特定フォルダ (`Projects/Work` のようなパス、または `folders` コマンドで表示される id) に入っているドキュメントをすべて取得し、再分類する。デフォルトではそのフォルダ自身も振り分け候補に残るため、本当にそこが適切なドキュメントはそのまま留まる。
- `--exclude-source-folder`: `--source-folder` と併用し、元のフォルダ自身を振り分け候補から除外する。指定すると、すべてのドキュメントが別のフォルダへ強制的に振り分けられる。
- `--min-confidence` (default `0.6`): この confidence 未満の判定は移動せずスキップする。
- `--limit`: 処理するドキュメント数の上限。
- `--sample-size` (default `5`): フォルダの説明文に使う既存ドキュメントのサンプル数。
- `--max-folders`: フォルダ数が多い場合に、ドキュメント数の多い上位N件に候補を絞る。
- `--max-content-chars` (default `4000`): 分類器に渡すドキュメント本文の最大文字数。

## Notes

- Craft の folders/documents API にはページネーションの記載がなく、一度のレスポンスで
  返る `items` をそのまま扱う。
- フォルダ数が多いスペースでは選択肢が多くなり TypeSafe のトークン消費が増えるため、
  `--max-folders` で上位フォルダに絞ることを推奨する。
- 移動は `RouteDocument` 単位 (1ドキュメントずつ) で行うため、途中でエラーが起きても
  他のドキュメントの処理は継続し、最後にまとめて `moved/skipped/failed` を報告する。
