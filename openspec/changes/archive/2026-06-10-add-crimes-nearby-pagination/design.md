# Design — Keyset pagination for crimes/nearby

## Sort key & cursor

Results are ordered by `(distance ASC, id ASC)`, where `distance = ST_Distance(geom::geography,
center::geography)` and `id` is the `crimes` primary key (stable tiebreaker for equal distances). The
cursor encodes the last row of the page: `{ "d": <distance>, "id": <id> }`, base64url-encoded
(`encoding/base64.RawURLEncoding` of compact JSON). The token is **opaque** — clients must not parse
it — so its internals can change later.

`id` is used only inside the cursor; it is not added to the crime record in the response (contract for
the items stays as-is).

## Keyset query

The distance is computed once in an inner query; the outer query applies the keyset predicate and the
limit. `ST_DWithin` keeps using the GiST index to bound candidates to the radius first.

```sql
SELECT source_id, year, month, day, date, hour, crime_type, crime_subtype,
       weapon_used, motorcycle_used, neighborhood, commune, quantity,
       longitude, latitude, id, distance
FROM (
  SELECT source_id, ..., to_char(date,'YYYY-MM-DD') AS date, ...,
         ST_X(geom) AS longitude, ST_Y(geom) AS latitude, id,
         ST_Distance(geom::geography, ST_SetSRID(ST_MakePoint($1,$2),4326)::geography) AS distance
  FROM crimes
  WHERE ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1,$2),4326)::geography, $3)
) c
WHERE $4::float8 IS NULL                       -- first page: no cursor
   OR c.distance > $4                          -- strictly farther, OR
   OR (c.distance = $4 AND c.id > $5)          -- same distance, later id
ORDER BY c.distance ASC, c.id ASC
LIMIT $6;
```

- Params: `$1=lng, $2=lat, $3=radius, $4=cursor.distance (nullable), $5=cursor.id (nullable), $6=limit+1`.
- **Fetch `limit + 1`**: if the DB returns more than `limit`, there is a next page → trim to `limit`,
  set `has_more=true`, and build `next_cursor` from the last *kept* row. Avoids a separate COUNT.
- Cursor round-trip is exact: `ST_Distance` is deterministic for the same geometry+center, and a
  `float64` survives JSON round-trip exactly, so `c.distance = $4` matches the boundary row reliably;
  the `id` tiebreaker covers equal-distance rows.

## Layering

- `cursor.go` (new): `Cursor{Distance, ID}` + `Encode()` / `DecodeCursor(string)`.
- Handler: parses `limit`/`cursor` strings → `ErrInvalidLimit` / `ErrInvalidCursor` (→ 400) on bad
  input; decodes the cursor into the domain `Cursor`.
- Service: applies defaults (`DefaultLimit=100`), validates (`1..MaxLimit=500`), calls the repo, and
  encodes `page.Next` into the opaque `next_cursor` token for the response DTO.
- Repository `FindNearby` returns a `CrimePage{ Items, HasMore, Next *Cursor }`.

`limit` out of range returns 400 (consistent with how `radius > 2000` is handled); clamping was the
alternative but we keep the codebase's reject-on-invalid convention.

## Response shape

```json
{ "lat": .., "lng": .., "radius_meters": 300, "count": 100,
  "items": [ ... ], "next_cursor": "eyJkIjouLi4sImlkIjouLn0", "has_more": true }
```

`count` = items in this page. `next_cursor` is `null` and `has_more` is `false` on the last page.
Empty result → `items: []`, `count: 0`, `has_more: false`, `next_cursor: null`.

## Testing

- Unit: service applies default limit, rejects invalid limit, passes cursor through, surfaces
  `has_more`/`next_cursor`; handler maps bad `limit`/`cursor` → 400.
- Integration (`//go:build integration`, live PostGIS): walk **all** pages around the Obelisco via
  `next_cursor`, assert (a) no duplicate `source_id` across pages, (b) non-decreasing distance across
  the whole walk, (c) the total walked equals a single unpaginated count within the radius.
