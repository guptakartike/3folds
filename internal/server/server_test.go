package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerIdleAndRunFlow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "threefolds-server-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	rootData := filepath.Join("..", "..", "data")
	for _, name := range []string{"settlements.json", "bank_statements.json", "ledger_entries.json"} {
		src := filepath.Join(rootData, name)
		if data, err := os.ReadFile(src); err == nil {
			os.WriteFile(filepath.Join(tmpDir, name), data, 0644)
		}
	}

	// 1. In Idle state (no match results yet):
	t.Run("Idle Overview", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/overview", nil)
		w := httptest.NewRecorder()
		handleOverview(tmpDir)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var res map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("unmarshaling overview response: %v", err)
		}

		if res["status"] != "idle" {
			t.Errorf("expected status 'idle', got %v", res["status"])
		}
		if res["has_run"] != false {
			t.Errorf("expected has_run false, got %v", res["has_run"])
		}
	})

	t.Run("Idle Audit Trail", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/audit-trail", nil)
		w := httptest.NewRecorder()
		handleAuditTrail(tmpDir)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var res map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &res)
		if res["status"] != "idle" {
			t.Errorf("expected audit-trail status 'idle', got %v", res["status"])
		}
		if res["has_run"] != false {
			t.Errorf("expected has_run false, got %v", res["has_run"])
		}
	})

	// 2. Run Reconciliation:
	t.Run("Run Reconciliation", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/run", nil)
		w := httptest.NewRecorder()
		handleRun(tmpDir)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var res runResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("unmarshaling run response: %v", err)
		}

		if res.Summary.Status != "populated" {
			t.Errorf("expected populated status, got %s", res.Summary.Status)
		}
		if !res.Summary.HasRun {
			t.Errorf("expected HasRun true in summary")
		}
		if res.Summary.Velocity < 90 {
			t.Errorf("expected velocity >= 90%%, got %.1f%%", res.Summary.Velocity)
		}
	})

	// 3. In Populated state:
	t.Run("Populated Overview and Audit Consistency", func(t *testing.T) {
		reqO := httptest.NewRequest("GET", "/api/overview", nil)
		wO := httptest.NewRecorder()
		handleOverview(tmpDir)(wO, reqO)

		var resO overviewResponse
		json.Unmarshal(wO.Body.Bytes(), &resO)

		if !resO.HasRun {
			t.Errorf("expected Overview HasRun true")
		}

		reqA := httptest.NewRequest("GET", "/api/audit-trail", nil)
		wA := httptest.NewRecorder()
		handleAuditTrail(tmpDir)(wA, reqA)

		var resA auditTrailResponse
		json.Unmarshal(wA.Body.Bytes(), &resA)

		if !resA.HasRun {
			t.Errorf("expected AuditTrail HasRun true")
		}

		// Assert volume matches across pages
		if resO.ProcessedVolume != resA.ReconciledVolume {
			t.Errorf("volume mismatch: overview has %s, audit trail has %s",
				resO.ProcessedVolume, resA.ReconciledVolume)
		}

		// Assert resolution time is non-zero
		if resO.Stats.AvgResolutionTime == "0.00ms / tx" || resO.Stats.AvgResolutionTime == "N/A" {
			t.Errorf("avg resolution time format is invalid: %s", resO.Stats.AvgResolutionTime)
		}
	})

	// 4. Reset to Idle:
	t.Run("Reset", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/reset", nil)
		w := httptest.NewRecorder()
		handleReset(tmpDir)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		// Verify overview is idle again
		reqO := httptest.NewRequest("GET", "/api/overview", nil)
		wO := httptest.NewRecorder()
		handleOverview(tmpDir)(wO, reqO)

		var resO map[string]interface{}
		json.Unmarshal(wO.Body.Bytes(), &resO)
		if resO["status"] != "idle" {
			t.Errorf("expected status 'idle' after reset, got %v", resO["status"])
		}
		if resO["has_run"] != false {
			t.Errorf("expected has_run false after reset, got %v", resO["has_run"])
		}
	})
}

func TestSampleEndpoints(t *testing.T) {
	for _, kind := range []string{"settlements", "bank-statements", "ledger"} {
		t.Run(kind+" CSV", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/sample/"+kind+"?format=csv", nil)
			w := httptest.NewRecorder()
			handleSample(kind)(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", w.Code)
			}
			if !strings.Contains(w.Header().Get("Content-Type"), "text/csv") {
				t.Errorf("expected text/csv content type, got %s", w.Header().Get("Content-Type"))
			}
		})

		t.Run(kind+" JSON", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/sample/"+kind+"?format=json", nil)
			w := httptest.NewRecorder()
			handleSample(kind)(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", w.Code)
			}
			if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
				t.Errorf("expected application/json content type, got %s", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestUploadParsingAndValidation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "threefolds-upload-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test 1: Uploading sample settlements CSV
	csvData := `settlement_id,payment_id,order_id,gross_amount_paisa,fee_paisa,tax_paisa,net_amount_paisa,settled_at,status
set_001,pay_001,order_001,100000,2000,360,97640,2026-02-01T10:00:00Z,settled
set_002,pay_002,order_002,200000,4000,720,195280,2026-02-02T10:00:00Z,settled`

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test_settlements.csv")
	part.Write([]byte(csvData))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/upload/settlements", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handleUpload(tmpDir, "settlements")(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var res uploadResponse
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Rows != 2 {
		t.Errorf("expected 2 rows, got %d", res.Rows)
	}
	if len(res.Preview) != 2 {
		t.Errorf("expected 2 preview items, got %d", len(res.Preview))
	}

	// Test 2: Uploading malformed CSV with missing headers
	badCSV := `id,amount,date
1,100,2026-01-01`
	body2 := &bytes.Buffer{}
	writer2 := multipart.NewWriter(body2)
	part2, _ := writer2.CreateFormFile("file", "bad.csv")
	part2.Write([]byte(badCSV))
	writer2.Close()

	req2 := httptest.NewRequest("POST", "/api/upload/settlements", body2)
	req2.Header.Set("Content-Type", writer2.FormDataContentType())
	w2 := httptest.NewRecorder()
	handleUpload(tmpDir, "settlements")(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad CSV, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "missing required column") {
		t.Errorf("expected error to mention missing required columns, got %s", w2.Body.String())
	}
}
