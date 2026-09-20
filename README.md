# craft-auto-routing

A CLI tool that reads the existing folder structure in Craft docs, has
[TypeSafe](https://docs.typesafe.ai)'s `jev` model classify unsorted
documents (Unsorted), and automatically routes each one to the most
appropriate folder.

## How it works

1. Fetch the existing folder tree from the Craft Connect API (`GET /folders`)
   and flatten it into folder paths (breadcrumbs like `Projects/Work`).
2. Sample the titles of documents actually contained in each folder
   (`GET /documents?folderId=...`) and build a description per folder that
   represents "what this folder contains." This is the part that reads the
   existing structure.
3. Fetch the list of documents with `location=unsorted`, and pass each
   document's body (`GET /blocks`) and title as the `state` of a `choice`
   question to the TypeSafe SystemOne API. The choices (`criteria`) are the
   per-folder descriptions built in step 2.
4. Look at the `choice` (folder ID) and `confidence` returned by
   `jev-latest`, and if it meets or exceeds the threshold, move the document
   to that folder via `PUT /documents/move`. If it's below the threshold,
   skip the move and report it instead.

## Setup

```sh
go build ./cmd/craft-auto-routing
```

Environment variables:

| Variable | Required | Description |
| --- | --- | --- |
| `CRAFT_API_TOKEN` | ✅ | Bearer token for the Craft Connect API |
| `CRAFT_BASE_URL` | ✅ | Your Craft Connect API base URL, e.g. `https://connect.craft.do/links/<your-link-id>/api/v1` (find this in Craft's Connect API settings for your space) |
| `TYPESAFE_API_KEY` | ✅ | TypeSafe API key |
| `TYPESAFE_MODEL` | - | Defaults to `jev-latest` |

Don't put tokens in `.env` or similar files; inject them from your shell's
secret management (`direnv`, keychain, etc.).

## Usage

```sh
# Inspect the existing folder structure and sample documents
craft-auto-routing folders

# Do a dry-run first to preview the routing plan
craft-auto-routing route --dry-run

# Actually move documents (skip anything below confidence 0.7)
craft-auto-routing route --min-confidence 0.7

# Re-classify everything already sitting in one folder, spreading it back out
craft-auto-routing route --source-folder "Projects/Inbox" --dry-run
```

Main flags (`route`):

- `--dry-run`: Only display the classification results without actually moving anything.
- `--location` (default `unsorted`): Craft location to pull documents from (`unsorted`, `trash`, `templates`, `daily_notes`). Mutually exclusive with `--source-folder`.
- `--source-folder`: Instead of `--location`, sweep every document already inside a specific real folder (accepts a folder path like `Projects/Work` or its id, as shown by `folders`) and re-classify each one. The source folder itself stays a candidate destination by default, so a document that genuinely still belongs there can be left in place.
- `--exclude-source-folder`: With `--source-folder`, remove the source folder from the candidate list, forcing every document out to a different folder.
- `--min-confidence` (default `0.6`): Skip moving documents whose confidence is below this value.
- `--limit`: Maximum number of documents to process.
- `--sample-size` (default `5`): Number of existing documents sampled to build each folder's description.
- `--max-folders`: When there are many folders, narrow the candidates to the top N by document count.
- `--max-content-chars` (default `4000`): Maximum number of characters of document body passed to the classifier.

## Notes

- The Craft folders/documents API doesn't document pagination, so the
  `items` returned in a single response are used as-is.
- In spaces with many folders, the number of choices increases, which
  increases TypeSafe token consumption; using `--max-folders` to narrow
  down to top folders is recommended.
- Moves are performed per `RouteDocument` (one document at a time), so if an
  error occurs partway through, processing continues for the remaining
  documents, and a summary of `moved/skipped/failed` is reported at the end.

## Releases

Pushing a `v*` tag runs [`.github/workflows/release.yml`](.github/workflows/release.yml), which:

- builds cross-platform binaries with [GoReleaser](https://goreleaser.com/) and attaches them to a GitHub Release, and
- builds and pushes a container image to `ghcr.io/ucpr/craft-auto-routing` with [`ko`](https://ko.build/), tagged with the tag name and `latest`.

## LICENSE

MIT LICENSE
