package crimes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerGetNearbyReturns400WhenLatIsMissing(t *testing.T) {
	handler := NewHandler(NewService(&fakeRepository{}))

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/crimes/nearby?lng=-58.4201&radius=300",
		nil,
	)
	response := httptest.NewRecorder()

	handler.GetNearby(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}

	if !strings.Contains(response.Body.String(), "lat and lng are required") {
		t.Fatalf("expected invalid coordinates message, got %s", response.Body.String())
	}
}

func TestHandlerGetNearbyReturns400WhenLngIsMissing(t *testing.T) {
	handler := NewHandler(NewService(&fakeRepository{}))

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/crimes/nearby?lat=-34.5895&radius=300",
		nil,
	)
	response := httptest.NewRecorder()

	handler.GetNearby(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}

	if !strings.Contains(response.Body.String(), "lat and lng are required") {
		t.Fatalf("expected invalid coordinates message, got %s", response.Body.String())
	}
}

func TestHandlerGetNearbyReturns400WhenRadiusIsNotANumber(t *testing.T) {
	handler := NewHandler(NewService(&fakeRepository{}))

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=abc",
		nil,
	)
	response := httptest.NewRecorder()

	handler.GetNearby(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}

	if !strings.Contains(response.Body.String(), "radius must be between 1 and 2000 meters") {
		t.Fatalf("expected invalid radius message, got %s", response.Body.String())
	}
}

func TestHandlerGetNearbyReturns200ForValidRequest(t *testing.T) {
	handler := NewHandler(NewService(&fakeRepository{
		items: []Crime{},
	}))

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300",
		nil,
	)
	response := httptest.NewRecorder()

	handler.GetNearby(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. body: %s", response.Code, response.Body.String())
	}

	if !strings.Contains(response.Body.String(), `"radius_meters":300`) {
		t.Fatalf("expected radius_meters 300, got %s", response.Body.String())
	}
}
