package roadgraph

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeService struct {
	stats GraphStats
	err   error
}

func (s *fakeService) GetStats(_ context.Context) (GraphStats, error) {
	return s.stats, s.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHandlerGetStats(t *testing.T) {
	tests := []struct {
		name       string
		svc        *fakeService
		wantStatus int
		wantBody   string
	}{
		{
			name:       "200 with stats JSON",
			svc:        &fakeService{stats: GraphStats{NodesCount: 71314, EdgesCount: 104309, WalkableEdges: 104309, RoutableEdges: 103570, ExcludedEdges: 739}},
			wantStatus: http.StatusOK,
			wantBody:   `"nodes_count":71314`,
		},
		{
			name:       "200 exposes routable and excluded edges",
			svc:        &fakeService{stats: GraphStats{EdgesCount: 104309, RoutableEdges: 103570, ExcludedEdges: 739}},
			wantStatus: http.StatusOK,
			wantBody:   `"routable_edges":103570`,
		},
		{
			name:       "200 with zeroed stats on empty graph",
			svc:        &fakeService{stats: GraphStats{}},
			wantStatus: http.StatusOK,
			wantBody:   `"excluded_edges":0`,
		},
		{
			name:       "500 when service errors",
			svc:        &fakeService{err: errors.New("db unavailable")},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "could not fetch road graph stats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(tt.svc, discardLogger())
			req := httptest.NewRequest(http.MethodGet, "/api/v1/roadgraph/stats", nil)
			rec := httptest.NewRecorder()

			handler.GetStats(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("want status %d, got %d — body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("want body to contain %q, got: %s", tt.wantBody, rec.Body.String())
			}
		})
	}
}
