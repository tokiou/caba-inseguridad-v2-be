# Crimes API — Delta

## ADDED Requirements

### Requirement: Keyset pagination

The nearby endpoint SHALL page results with a keyset cursor ordered by `(distance, id)`. It accepts a
`limit` (page size, default 100, max 500) and an optional opaque `cursor`. The response SHALL include
`next_cursor` (the opaque token for the next page, `null` when there are no more results) and
`has_more`. `count` SHALL be the number of items in the current page.

#### Scenario: First page

- GIVEN a valid nearby query with `limit=100` and no `cursor`
- WHEN the endpoint is queried and more than 100 matches exist within the radius
- THEN at most 100 items are returned, ordered nearest-first
- AND `has_more` is `true` and `next_cursor` is a non-null opaque token

#### Scenario: Following a cursor

- GIVEN a `next_cursor` returned by a previous page
- WHEN the endpoint is queried again with the same `lat`/`lng`/`radius` and that `cursor`
- THEN the next page continues strictly after the previous page's last item (no duplicates, no gaps)
- AND distances are non-decreasing across page boundaries

#### Scenario: Last page

- WHEN the final page is returned
- THEN `has_more` is `false` and `next_cursor` is `null`

#### Scenario: Invalid limit or cursor

- GIVEN `limit` that is non-numeric, less than 1, or greater than 500, OR a `cursor` that is not a
  valid token
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

## MODIFIED Requirements

### Requirement: Nearby crimes endpoint

The API SHALL expose `GET /api/v1/crimes/nearby` accepting `lat` and `lng`, an optional `radius`
(meters), an optional `limit` (page size), and an optional `cursor`, returning a page of crimes near
the point ordered nearest-first.

#### Scenario: Successful nearby query

- GIVEN valid CABA coordinates `lat` and `lng`
- WHEN a client sends `GET /api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300&limit=100`
- THEN the response is HTTP 200
- AND the body contains `lat`, `lng`, `radius_meters`, `count`, `items`, `next_cursor`, and `has_more`
- AND `items` holds at most `limit` crime records ordered nearest-first

#### Scenario: Default radius and limit

- GIVEN a request with no `radius` and no `limit`
- WHEN the endpoint is queried
- THEN a default radius of 300 meters and a default limit of 100 are applied
