package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthy_noConnection(t *testing.T) {
	a := &App{}
	require.False(t, a.Healthy())
}

func TestHandleHealth_degradedWithoutConnection(t *testing.T) {
	a := &App{}
	a.ready.Store(true)

	rr := httptest.NewRecorder()
	a.handleHealth(rr, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil))

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	require.Equal(t, "degraded", body["status"])
}
