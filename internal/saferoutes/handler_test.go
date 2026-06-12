package saferoutes

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

type stubService struct {
	response SafeRoutesResponse
	err      error
}

func (s *stubService) SafeRoutes(context.Context, SafeRoutesQuery) (SafeRoutesResponse, error) {
	return s.response, s.err
}

func newTestServer(svc service) *httptest.Server {
	r := chi.NewRouter()
	NewHandler(svc, slog.New(slog.DiscardHandler)).Register(r)
	return httptest.NewServer(r)
}

const validParams = "origin_lat=-34.58&origin_lng=-58.42&dest_lat=-34.60&dest_lng=-58.38"

func TestGetSafeRoutesOK(t *testing.T) {
	svc := &stubService{response: SafeRoutesResponse{
		TimeBucket:  "night",
		WeekdayType: "weekday",
		Routes: []SafeRoute{
			{Kind: "fastest"}, {Kind: "balanced"}, {Kind: "safest"}, {Kind: "least_safe_candidate"},
		},
	}}
	server := newTestServer(svc)
	defer server.Close()

	resp, err := http.Get(server.URL + "/routes/safe?" + validParams +
		"&datetime=2026-06-12T23:00:00-03:00")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body SafeRoutesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Routes) != 4 || body.TimeBucket != "night" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestGetSafeRoutesBadParams(t *testing.T) {
	server := newTestServer(&stubService{})
	defer server.Close()

	cases := []string{
		"",
		"origin_lat=-34.58",
		"origin_lat=abc&origin_lng=-58.42&dest_lat=-34.60&dest_lng=-58.38",
		validParams + "&datetime=not-a-date",
	}
	for _, params := range cases {
		resp, err := http.Get(server.URL + "/routes/safe?" + params)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("params %q: status = %d, want 400", params, resp.StatusCode)
		}
	}
}

func TestGetSafeRoutesErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid coordinates", ErrInvalidCoordinates, http.StatusBadRequest, "invalid_request"},
		{"outside graph", ErrPointOutsideGraph, http.StatusBadRequest, "origin_or_destination_outside_walkable_graph"},
		{"no route", ErrNoRoute, http.StatusNotFound, "route_not_found"},
		{"no active model", ErrNoActiveModel, http.StatusServiceUnavailable, "risk_model_unavailable"},
		{"internal", context.DeadlineExceeded, http.StatusInternalServerError, "internal_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := newTestServer(&stubService{err: c.err})
			defer server.Close()

			resp, err := http.Get(server.URL + "/routes/safe?" + validParams)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != c.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, c.wantStatus)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error != c.wantCode {
				t.Fatalf("error code = %q, want %q", body.Error, c.wantCode)
			}
		})
	}
}
