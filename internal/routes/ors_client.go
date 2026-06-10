package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ORSClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewORSClient(apiKey, baseURL string, httpClient *http.Client) *ORSClient {
	return &ORSClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

type orsResponse struct {
	Features []struct {
		Geometry struct {
			Type        string      `json:"type"`
			Coordinates [][]float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties struct {
			Summary struct {
				Distance float64 `json:"distance"`
				Duration float64 `json:"duration"`
			} `json:"summary"`
		} `json:"properties"`
	} `json:"features"`
}

func (c *ORSClient) FetchRoute(ctx context.Context, query RouteQuery) (Route, error) {
	url := fmt.Sprintf(
		"%s/v2/directions/%s?start=%f,%f&end=%f,%f",
		c.baseURL, query.Profile,
		query.OriginLng, query.OriginLat,
		query.DestLng, query.DestLat,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Route{}, fmt.Errorf("routes: build request: %w", ErrExternalService)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	// ORS's GET directions endpoint only serves GeoJSON; "application/json" yields a 406.
	req.Header.Set("Accept", "application/geo+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Route{}, fmt.Errorf("routes: ORS unreachable: %w", ErrExternalService)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// Capture a bounded slice of the body so the wrapped error carries ORS's
		// own diagnostic (e.g. an auth or rate-limit message) into the logs.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		detail := strings.TrimSpace(string(body))

		// ORS returns 404 only when no route exists between the points. Every
		// other 4xx/5xx (401/403 auth, 429 rate limit, 400 bad request, 5xx) is
		// an upstream or configuration problem — NOT a missing route.
		if resp.StatusCode == http.StatusNotFound {
			return Route{}, fmt.Errorf("routes: ORS found no route (status %d): %s: %w", resp.StatusCode, detail, ErrRouteNotFound)
		}
		return Route{}, fmt.Errorf("routes: ORS request failed (status %d): %s: %w", resp.StatusCode, detail, ErrExternalService)
	}

	var orsResp orsResponse
	if err := json.NewDecoder(resp.Body).Decode(&orsResp); err != nil {
		return Route{}, fmt.Errorf("routes: decode ORS response: %w", ErrExternalService)
	}

	if len(orsResp.Features) == 0 {
		return Route{}, ErrRouteNotFound
	}

	f := orsResp.Features[0]
	return Route{
		Distance: f.Properties.Summary.Distance,
		Duration: f.Properties.Summary.Duration,
		Geometry: GeoJSONLineString{
			Type:        f.Geometry.Type,
			Coordinates: f.Geometry.Coordinates,
		},
	}, nil
}
