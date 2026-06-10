# Proposal — Keyset pagination for crimes/nearby

## Why

`GET /api/v1/crimes/nearby` currently returns **every** crime within the radius (e.g. ~11,278 within
300m of the Obelisco → a 300KB+ response). That does not scale: large payloads, slow serialization,
and no way for a client to page through results. We add pagination so a client fetches a bounded page
and walks forward as needed.

## What

- Add **keyset (cursor) pagination** ordered by `(distance, id)`:
  - `limit` query param — page size (default 100, max 500).
  - `cursor` query param — opaque token (base64url of `{distance, id}`) marking where the next page
    starts. Absent on the first request.
- Response gains `next_cursor` (opaque token, `null` when there are no more results) and `has_more`.
  `count` becomes the number of items **in this page**.
- Keyset (not offset): no `OFFSET` scan penalty on deep pages, and stable under concurrent inserts.

## In scope

- `internal/crimes`: cursor codec, query/response DTOs, service validation + defaults, pgx keyset
  query, handler param parsing.
- Unit tests + the build-tagged integration test (full walk-through-all-pages correctness).

## Out of scope

- Total-count of matches (keyset deliberately avoids the expensive `COUNT`; use `has_more`).
- Random page access ("jump to page N") — not supported by keyset; not needed for nearest-crimes UX.
- Changing radius/coordinate validation or the crime record shape.

## Contract change

Additive: existing clients that ignore the new params still work, but now receive at most `limit`
(default 100) items instead of all. New response fields `next_cursor` and `has_more` are added.
