package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

func (c *ORSClient) GetRoute(ctx context.Context, query RouteQuery) (Route, error) {
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
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Route{}, fmt.Errorf("routes: ORS unreachable: %w", ErrExternalService)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return Route{}, fmt.Errorf("routes: ORS server error %d: %w", resp.StatusCode, ErrExternalService)
	}
	if resp.StatusCode >= 400 {
		return Route{}, fmt.Errorf("routes: ORS no route %d: %w", resp.StatusCode, ErrRouteNotFound)
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
