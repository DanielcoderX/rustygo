package rustygo_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rg "rustygo"
)

func TestDebugHandlerHTMLAndJSON(t *testing.T) {
	rg.GlobalAutoTuner.Calibrate("http_worker_endpoint", 32*1024)

	// Test HTML Output
	reqHTML := httptest.NewRequest(http.MethodGet, "/debug/rustygo/arenas", nil)
	recHTML := httptest.NewRecorder()
	rg.DebugHandler().ServeHTTP(recHTML, reqHTML)

	if recHTML.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recHTML.Code)
	}
	body := recHTML.Body.String()
	if !strings.Contains(body, "RustyGo Runtime Arena Inspector") {
		t.Fatalf("missing inspector title in html")
	}
	if !strings.Contains(body, "http_worker_endpoint") {
		t.Fatalf("missing calibrated call site in html")
	}

	// Test JSON Output
	reqJSON := httptest.NewRequest(http.MethodGet, "/debug/rustygo/arenas?format=json", nil)
	recJSON := httptest.NewRecorder()
	rg.DebugHandler().ServeHTTP(recJSON, reqJSON)

	if recJSON.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recJSON.Code)
	}

	var data rg.DebugDashboardData
	if err := json.Unmarshal(recJSON.Body.Bytes(), &data); err != nil {
		t.Fatalf("failed unmarshaling json response: %v", err)
	}

	if data.AutoTuner["http_worker_endpoint"] != 32*1024 {
		t.Fatalf("expected calibration 32768, got %d", data.AutoTuner["http_worker_endpoint"])
	}
}

func TestRegisterDebugHandler(t *testing.T) {
	mux := http.NewServeMux()
	rg.RegisterDebugHandler(mux)

	req := httptest.NewRequest(http.MethodGet, "/debug/rustygo/arenas", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
