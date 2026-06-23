# Safe Routes — Delta Specification

## ADDED Requirements

### Requirement: Safe routes requires authentication

`GET /api/v1/routes/safe` SHALL require a valid access token. The endpoint SHALL be mounted behind the
auth middleware so that requests without a valid `Authorization: Bearer <access_token>` are rejected
before any routing or risk work is done. The other open-data endpoints (`/crimes/nearby`, `/routes`,
`/roadgraph/*`) remain public.

#### Scenario: Unauthenticated safe-route request is rejected

- WHEN a client calls `GET /api/v1/routes/safe` with no or an invalid/expired access token
- THEN the response is `401` with error code `unauthorized`
- AND no route or risk computation is performed

#### Scenario: Authenticated safe-route request succeeds

- GIVEN a valid access token for an active user
- WHEN the client calls `GET /api/v1/routes/safe` with `Authorization: Bearer <token>` and valid
  origin/destination parameters
- THEN the request is processed and returns the safe-route response as specified
