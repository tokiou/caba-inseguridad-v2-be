# Tasks — Keyset pagination for crimes/nearby

1. `internal/crimes/cursor.go` — `Cursor{Distance, ID}` + `Encode()` / `DecodeCursor(string)`
   (base64url of compact JSON).
2. `internal/crimes/errors.go` — add `ErrInvalidLimit`, `ErrInvalidCursor`.
3. `internal/crimes/dto.go` — `NearbyCrimesQuery` gets `Limit int`, `Cursor *Cursor`;
   `NearbyCrimesResponse` gets `NextCursor *string` (`json:"next_cursor"`) and `HasMore bool`
   (`json:"has_more"`).
4. `internal/crimes/repository.go` — `CrimePage{Items, HasMore, Next *Cursor}`; keyset query
   (fetch `limit+1`, trim, build next cursor); `FindNearby` returns `CrimePage`.
5. `internal/crimes/service.go` — `DefaultLimit=100`, `MaxLimit=500`; default + validate limit;
   encode `page.Next` into `NextCursor`; set `HasMore`.
6. `internal/crimes/handler.go` — parse `limit`/`cursor`; map `ErrInvalidLimit`/`ErrInvalidCursor`
   → 400.
7. Tests — update `service_test.go` (mock returns `CrimePage`; new cases) and `handler_test.go`
   (invalid limit/cursor → 400; success exposes `next_cursor`); extend
   `repository_integration_test.go` with a full multi-page walk.
8. `go build ./...`, `go vet ./...`, `go test ./...`, `go test -tags=integration ./internal/crimes/...`.
9. Archive: merge delta into `openspec/specs/crimes-api/spec.md`; move change folder to
   `openspec/changes/archive/2026-06-10-add-crimes-nearby-pagination/`.
