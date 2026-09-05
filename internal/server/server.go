// Package server provides the HTTP API for the 3folds reconciliation
// dashboard, serving real data from the data directory to the React
// frontend.
package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"threefolds/internal/evaluation"
	"threefolds/internal/generator"
	"threefolds/internal/loader"
	"threefolds/internal/matcher"
	"threefolds/internal/model"
	"threefolds/internal/report"
	"threefolds/internal/resolver"
)

// Run starts the HTTP server on the given port, serving data from dataDir.
func Run(port int, dataDir string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/overview", handleOverview(dataDir))
	mux.HandleFunc("GET /api/exceptions", handleExceptions(dataDir))
	mux.HandleFunc("GET /api/audit-trail", handleAuditTrail(dataDir))

	mux.HandleFunc("POST /api/upload/settlements", handleUpload(dataDir, "settlements"))
	mux.HandleFunc("POST /api/upload/bank-statements", handleUpload(dataDir, "bank-statements"))
	mux.HandleFunc("POST /api/upload/ledger", handleUpload(dataDir, "ledger"))

	mux.HandleFunc("GET /api/sample/settlements", handleSample("settlements"))
	mux.HandleFunc("GET /api/sample/bank-statements", handleSample("bank-statements"))
	mux.HandleFunc("GET /api/sample/ledger", handleSample("ledger"))

	mux.HandleFunc("POST /api/run", handleRun(dataDir))
	mux.HandleFunc("POST /api/reset", handleReset(dataDir))
	mux.HandleFunc("DELETE /api/reset", handleReset(dataDir))

	handler := corsMiddleware(mux)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("3folds API server listening on http://localhost%s", addr)
	log.Printf("data directory: %s", dataDir)

	return http.ListenAndServe(addr, handler)
}

// ── CORS middleware ─────────────────────────────────────────────────────

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ── JSON helpers ────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSONFile(path string, v interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}

func writeJSONFile(path string, v interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ── Types for API responses ─────────────────────────────────────────────

type evaluationInfo struct {
	HasEvaluation      bool    `json:"hasEvaluation"`
	Total              int     `json:"total"`
	Correct            int     `json:"correct"`
	Wrong              int     `json:"wrong"`
	Accuracy           float64 `json:"accuracy"`
	ExpectedMatches    int     `json:"expectedMatches"`
	Matched            int     `json:"matched"`
	MatchCoverage      float64 `json:"matchCoverage"`
	ExpectedExceptions int     `json:"expectedExceptions"`
	DetectedExceptions int     `json:"detectedExceptions"`
	ExceptionDetection float64 `json:"exceptionDetection"`
	FalsePositives     int     `json:"falsePositives"`
	FalseNegatives     int     `json:"falseNegatives"`
}

type overviewResponse struct {
	Status             string          `json:"status"`
	HasRun             bool            `json:"has_run"`
	LastUpdated        string          `json:"lastUpdated"`
	SyncDescription    string          `json:"syncDescription"`
	ReconciliationRate float64         `json:"reconciliationRate"`
	Velocity           float64         `json:"velocity"`
	MatchedCount       int             `json:"matchedCount"`
	DeterministicCount int             `json:"deterministicCount"`
	AIResolvedCount    int             `json:"aiResolvedCount"`
	TotalCount         int             `json:"totalCount"`
	NeedReview         int             `json:"needReview"`
	ProcessedVolume    string          `json:"processedVolume"`
	BatchID            string          `json:"batchId"`
	Tiers              overviewTiers   `json:"tiers"`
	Stats              overviewStats   `json:"stats"`
	Evaluation         *evaluationInfo `json:"evaluation,omitempty"`
}

type overviewTiers struct {
	Exact      tierInfo `json:"exact"`
	Fuzzy      tierInfo `json:"fuzzy"`
	Batch      tierInfo `json:"batch"`
	AIResolved tierInfo `json:"aiResolved"`
	Unresolved tierInfo `json:"unresolved"`
}

type tierInfo struct {
	Label string  `json:"label"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
	Color string  `json:"color"`
}

type overviewStats struct {
	TotalTransactions  int    `json:"totalTransactions"`
	AvgResolutionTime  string `json:"avgResolutionTime"`
	ExceptionsThisWeek int    `json:"exceptionsThisWeek"`
}

type exceptionsResponse struct {
	Status          string         `json:"status"`
	HasRun          bool           `json:"has_run"`
	DisputedVolume  string         `json:"disputedVolume"`
	CriticalCount   int            `json:"criticalCount"`
	OpenCount       int            `json:"openCount"`
	FuzzyCount      int            `json:"fuzzyCount"`
	UnresolvedCount int            `json:"unresolvedCount"`
	Rows            []exceptionRow `json:"rows"`
}

type exceptionRow struct {
	ID             int              `json:"id"`
	OrderID        string           `json:"orderId"`
	SettlementID   string           `json:"settlementId"`
	Tier           string           `json:"tier"`
	TierColor      string           `json:"tierColor"`
	ReviewType     string           `json:"reviewType"`
	Label          string           `json:"label"`
	Description    string           `json:"description"`
	Amount         string           `json:"amount"`
	Delta          *string          `json:"delta"`
	DeltaNote      *string          `json:"deltaNote"`
	DeltaNoteColor *string          `json:"deltaNoteColor"`
	Expanded       bool             `json:"expanded"`
	Detail         *exceptionDetail `json:"detail,omitempty"`
}

type exceptionDetail struct {
	Left         detailPanel `json:"left"`
	Right        detailPanel `json:"right"`
	Chips        []chip      `json:"chips"`
	AISuggestion string      `json:"aiSuggestion"`
}

type detailPanel struct {
	Header    string     `json:"header"`
	Timestamp string     `json:"timestamp"`
	Rows      []labelVal `json:"rows"`
}

type labelVal struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type chip struct {
	Icon string `json:"icon"`
	Text string `json:"text"`
}

type auditTrailResponse struct {
	Status             string          `json:"status"`
	HasRun             bool            `json:"has_run"`
	ReconciledVolume   string          `json:"reconciledVolume"`
	ParityIndex        float64         `json:"parityIndex"`
	ReconciliationRate float64         `json:"reconciliationRate"`
	ExactCount         int             `json:"exactCount"`
	FuzzyCount         int             `json:"fuzzyCount"`
	BatchCount         int             `json:"batchCount"`
	AICount            int             `json:"aiCount"`
	DeterministicCount int             `json:"deterministicCount"`
	UnresolvedCount    int             `json:"unresolvedCount"`
	ReconciledCount    int             `json:"reconciledCount"`
	TotalCount         int             `json:"totalCount"`
	Rows               []auditRow      `json:"rows"`
	Evaluation         *evaluationInfo `json:"evaluation,omitempty"`
}

type auditRow struct {
	OrderID         string      `json:"orderId"`
	SettlementID    string      `json:"settlementId"`
	Tier            string      `json:"tier"`
	TierColor       string      `json:"tierColor"`
	BankRef         string      `json:"bankRef"`
	Amount          string      `json:"amount"`
	AmountDiff      string      `json:"amountDiff"`
	AmountDiffColor *string     `json:"amountDiffColor"`
	DateDiff        string      `json:"dateDiff"`
	DateDiffColor   *string     `json:"dateDiffColor"`
	Reason          string      `json:"reason"`
	ReasonHighlight interface{} `json:"reasonHighlight"`
}

func loadEvaluation(dataDir string, results []matcher.Result) *evaluationInfo {
	truthPath := filepath.Join(dataDir, "ground_truth.json")
	data, err := os.ReadFile(truthPath)
	if err != nil {
		return nil
	}
	var truth []evaluation.GroundTruth
	if err := json.Unmarshal(data, &truth); err != nil {
		return nil
	}
	eval := evaluation.Calculate(truth, results, 0)
	return &evaluationInfo{
		HasEvaluation:      true,
		Total:              eval.Total,
		Correct:            eval.Correct,
		Wrong:              eval.Wrong,
		Accuracy:           eval.Accuracy,
		ExpectedMatches:    eval.ExpectedMatches,
		Matched:            eval.Matched,
		MatchCoverage:      eval.MatchCoverage,
		ExpectedExceptions: eval.ExpectedExceptions,
		DetectedExceptions: eval.DetectedExceptions,
		ExceptionDetection: eval.ExceptionDetection,
		FalsePositives:     eval.FalsePositives,
		FalseNegatives:     eval.FalseNegatives,
	}
}

// ── GET /api/overview ───────────────────────────────────────────────────

func handleOverview(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, _, err := report.Load(dataDir)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, overviewResponse{
					Status:  "idle",
					HasRun:  false,
					BatchID: "-",
				})
				return
			}
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("loading results: %v", err))
			return
		}

		if len(results) == 0 {
			writeJSON(w, http.StatusOK, overviewResponse{
				Status:  "idle",
				HasRun:  false,
				BatchID: "-",
			})
			return
		}

		summary := report.Summarize(results)

		// Read metrics for processing time.
		var metrics struct {
			ProcessingTimeMs float64 `json:"processing_time_ms"`
			RecordsPerSecond float64 `json:"records_per_second"`
		}
		metricsPath := filepath.Join(dataDir, "metrics.json")
		if data, err := os.ReadFile(metricsPath); err == nil {
			json.Unmarshal(data, &metrics)
		}

		// Read settlements for processed volume.
		var settlements []model.Settlement
		readJSONFile(filepath.Join(dataDir, "settlements.json"), &settlements)

		var totalNetPaisa int64
		for _, s := range settlements {
			totalNetPaisa += s.NetAmountPaisa
		}
		processedVolume := formatINR(float64(totalNetPaisa) / 100)

		// Compute avg resolution time honestly with sub-millisecond precision.
		avgResTime := "< 1µs / tx"
		if metrics.ProcessingTimeMs > 0 && summary.Total > 0 {
			avgMs := metrics.ProcessingTimeMs / float64(summary.Total)
			if avgMs < 0.01 {
				avgMicroseconds := avgMs * 1000
				avgResTime = fmt.Sprintf("%.1fµs / tx", avgMicroseconds)
			} else if avgMs < 1 {
				avgResTime = fmt.Sprintf("%.2fms / tx", avgMs)
			} else {
				avgResTime = fmt.Sprintf("%.1fms / tx", avgMs)
			}
		}

		deterministicCount := summary.Exact + summary.Fuzzy + summary.Batch
		aiResolvedCount := summary.LLMResolved
		matchedCount := deterministicCount + aiResolvedCount
		total := summary.Total
		reconciliationRate := math.Round(summary.MatchRatePct*10) / 10

		pct := func(count int) float64 {
			if total == 0 {
				return 0
			}
			return math.Round(float64(count)/float64(total)*1000) / 10
		}

		eval := loadEvaluation(dataDir, results)

		resp := overviewResponse{
			Status:             "populated",
			HasRun:             true,
			LastUpdated:        time.Now().Format("Today at 3:04 PM"),
			SyncDescription:    "Live reconciliation benchmark results",
			ReconciliationRate: reconciliationRate,
			Velocity:           reconciliationRate,
			MatchedCount:       matchedCount,
			DeterministicCount: deterministicCount,
			AIResolvedCount:    aiResolvedCount,
			TotalCount:         total,
			NeedReview:         summary.Exceptions,
			ProcessedVolume:    processedVolume,
			BatchID:            "#LIVE-BENCHMARK",
			Tiers: overviewTiers{
				Exact: tierInfo{
					Label: "EXACT MATCH",
					Count: summary.Exact,
					Pct:   pct(summary.Exact),
					Color: "green",
				},
				Fuzzy: tierInfo{
					Label: "FUZZY MATCH",
					Count: summary.Fuzzy,
					Pct:   pct(summary.Fuzzy),
					Color: "lavender",
				},
				Batch: tierInfo{
					Label: "BATCH RECONCILIATION",
					Count: summary.Batch,
					Pct:   pct(summary.Batch),
					Color: "blue",
				},
				AIResolved: tierInfo{
					Label: "AI-RESOLVED",
					Count: summary.LLMResolved,
					Pct:   pct(summary.LLMResolved),
					Color: "purple",
				},
				Unresolved: tierInfo{
					Label: "UNRESOLVED (EXCEPTIONS)",
					Count: summary.Exceptions,
					Pct:   pct(summary.Exceptions),
					Color: "red",
				},
			},
			Stats: overviewStats{
				TotalTransactions:  total,
				AvgResolutionTime:  avgResTime,
				ExceptionsThisWeek: summary.Exceptions,
			},
			Evaluation: eval,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ── GET /api/exceptions ─────────────────────────────────────────────────

func handleExceptions(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, _, err := report.Load(dataDir)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, exceptionsResponse{
					Status:         "idle",
					HasRun:         false,
					DisputedVolume: "₹0.00",
					CriticalCount:  0,
					OpenCount:      0,
					Rows:           []exceptionRow{},
				})
				return
			}
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("loading results: %v", err))
			return
		}

		if len(results) == 0 {
			writeJSON(w, http.StatusOK, exceptionsResponse{
				Status:         "idle",
				HasRun:         false,
				DisputedVolume: "₹0.00",
				CriticalCount:  0,
				OpenCount:      0,
				Rows:           []exceptionRow{},
			})
			return
		}

		// Read settlements to get amounts.
		var settlements []model.Settlement
		readJSONFile(filepath.Join(dataDir, "settlements.json"), &settlements)
		settlementMap := make(map[string]model.Settlement)
		for _, s := range settlements {
			settlementMap[s.SettlementID] = s
		}

		var rows []exceptionRow
		var totalDisputedPaisa int64
		id := 0
		criticalCount := 0
		fuzzyCount := 0
		unresolvedCount := 0

		for _, res := range results {
			if res.Tier != matcher.TierUnresolved && res.Tier != matcher.TierFuzzy {
				continue
			}

			id++

			s := settlementMap[res.SettlementID]
			netINR := float64(s.NetAmountPaisa) / 100
			grossINR := float64(s.GrossAmountPaisa) / 100
			feeINR := float64(s.FeePaisa) / 100
			taxINR := float64(s.TaxPaisa) / 100

			totalDisputedPaisa += s.NetAmountPaisa

			// Determine tier label, reviewType and color.
			reviewType := "unresolved"
			tier := "UNRESOLVED"
			tierColor := "red"
			label := "Unresolved:"
			if res.Tier == matcher.TierFuzzy {
				reviewType = "fuzzy"
				tier = "FUZZY REVIEW"
				tierColor = "lavender"
				label = "Fuzzy match review:"
				fuzzyCount++
			} else {
				unresolvedCount++
				criticalCount++
			}

			// Derive label from reason.
			if res.BankUTRRef == "" && res.Tier == matcher.TierUnresolved {
				label = "Missing bank record:"
			} else if res.AmountDiffINR > 0 {
				label = "Amount discrepancy:"
			}

			// Format amount and delta.
			amount := formatINR(netINR)
			var delta *string
			if res.AmountDiffINR != 0 {
				d := fmt.Sprintf("Δ %s", formatINRSigned(-res.AmountDiffINR))
				delta = &d
			}

			var deltaNote *string
			var deltaNoteColor *string
			if res.Tier == matcher.TierUnresolved && res.BankUTRRef == "" {
				note := "No UTR record"
				deltaNote = &note
				c := "red"
				deltaNoteColor = &c
			} else if res.DateDiffHours > 72 {
				note := fmt.Sprintf("+%.0f hrs elapsed", res.DateDiffHours)
				deltaNote = &note
				c := "red"
				deltaNoteColor = &c
			}

			// Build detail panel for expanded view.
			var detail *exceptionDetail
			settledAt := "-"
			if !s.SettledAt.IsZero() {
				settledAt = s.SettledAt.Format("02 Jan 2006, 15:04 IST")
			}

			detail = &exceptionDetail{
				Left: detailPanel{
					Header:    "RAZORPAY SETTLEMENT PAYLOAD",
					Timestamp: settledAt,
					Rows: []labelVal{
						{Label: "Gross Transaction Value", Value: formatINR(grossINR)},
						{Label: "Razorpay Fee", Value: formatINR(feeINR)},
						{Label: "Tax (GST)", Value: formatINR(taxINR)},
						{Label: "Calculated Settlement Net", Value: formatINR(netINR)},
					},
				},
				Right: detailPanel{
					Header:    "BANK MATCH STATUS",
					Timestamp: "-",
					Rows:      []labelVal{},
				},
				Chips: []chip{
					{Icon: "amount", Text: fmt.Sprintf("Amount diff: %s", formatINR(res.AmountDiffINR))},
					{Icon: "time", Text: fmt.Sprintf("Date diff: %.0f hrs", res.DateDiffHours)},
				},
				AISuggestion: "Review and reconcile manually based on available evidence",
			}

			if res.BankUTRRef != "" {
				detail.Right.Rows = append(detail.Right.Rows,
					labelVal{Label: "UTR Reference", Value: res.BankUTRRef},
				)
				detail.Chips = append(detail.Chips,
					chip{Icon: "bank", Text: fmt.Sprintf("Bank Ref: %s", res.BankUTRRef)},
				)
			} else {
				detail.Right.Rows = append(detail.Right.Rows,
					labelVal{Label: "Status", Value: "No matching bank record found"},
				)
			}

			if res.LedgerFound {
				detail.Right.Rows = append(detail.Right.Rows,
					labelVal{Label: "Ledger Status", Value: res.LedgerStatus},
				)
			}

			row := exceptionRow{
				ID:             id,
				OrderID:        res.OrderID,
				SettlementID:   res.SettlementID,
				Tier:           tier,
				TierColor:      tierColor,
				ReviewType:     reviewType,
				Label:          label,
				Description:    res.Reason,
				Amount:         amount,
				Delta:          delta,
				DeltaNote:      deltaNote,
				DeltaNoteColor: deltaNoteColor,
				Expanded:       id == 1,
				Detail:         detail,
			}

			rows = append(rows, row)
		}

		if rows == nil {
			rows = []exceptionRow{}
		}

		resp := exceptionsResponse{
			Status:          "populated",
			HasRun:          true,
			DisputedVolume:  formatINR(float64(totalDisputedPaisa) / 100),
			CriticalCount:   criticalCount,
			OpenCount:       len(rows),
			FuzzyCount:      fuzzyCount,
			UnresolvedCount: unresolvedCount,
			Rows:            rows,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ── GET /api/audit-trail ────────────────────────────────────────────────

func handleAuditTrail(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, _, err := report.Load(dataDir)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, auditTrailResponse{
					Status:           "idle",
					HasRun:           false,
					ReconciledVolume: "₹0.00",
					ParityIndex:      0,
					ExactCount:       0,
					FuzzyCount:       0,
					BatchCount:       0,
					AICount:          0,
					UnresolvedCount:  0,
					TotalCount:       0,
					Rows:             []auditRow{},
				})
				return
			}
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("loading results: %v", err))
			return
		}

		if len(results) == 0 {
			writeJSON(w, http.StatusOK, auditTrailResponse{
				Status:           "idle",
				HasRun:           false,
				ReconciledVolume: "₹0.00",
				ParityIndex:      0,
				ExactCount:       0,
				FuzzyCount:       0,
				BatchCount:       0,
				AICount:          0,
				UnresolvedCount:  0,
				TotalCount:       0,
				Rows:             []auditRow{},
			})
			return
		}

		// Read settlements to get transaction amounts.
		var settlements []model.Settlement
		readJSONFile(filepath.Join(dataDir, "settlements.json"), &settlements)
		settlementMap := make(map[string]model.Settlement)
		var totalNetPaisa int64
		for _, s := range settlements {
			settlementMap[s.SettlementID] = s
			totalNetPaisa += s.NetAmountPaisa
		}

		summary := report.Summarize(results)
		reconciledVolume := formatINR(float64(totalNetPaisa) / 100)
		deterministicCount := summary.Exact + summary.Fuzzy + summary.Batch
		aiResolvedCount := summary.LLMResolved
		reconciledCount := deterministicCount + aiResolvedCount
		reconciliationRate := math.Round(summary.MatchRatePct*10) / 10

		rows := make([]auditRow, 0, len(results))

		for _, res := range results {
			tier, tierColor := mapTier(res.Tier)

			bankRef := res.BankUTRRef
			if bankRef == "" {
				bankRef = "-"
			}

			s := settlementMap[res.SettlementID]
			amount := formatINR(float64(s.NetAmountPaisa) / 100)

			amountDiff := formatINR(res.AmountDiffINR)
			if res.AmountDiffINR > 0 {
				amountDiff = "+" + amountDiff
			}

			var amountDiffColor *string
			if res.AmountDiffINR != 0 {
				c := "red"
				amountDiffColor = &c
			}

			dateDiff := fmt.Sprintf("%.0fh", res.DateDiffHours)

			var dateDiffColor *string
			if res.DateDiffHours > 72 {
				c := "red"
				dateDiffColor = &c
			}

			rows = append(rows, auditRow{
				OrderID:         res.OrderID,
				SettlementID:    res.SettlementID,
				Tier:            tier,
				TierColor:       tierColor,
				BankRef:         bankRef,
				Amount:          amount,
				AmountDiff:      amountDiff,
				AmountDiffColor: amountDiffColor,
				DateDiff:        dateDiff,
				DateDiffColor:   dateDiffColor,
				Reason:          res.Reason,
				ReasonHighlight: nil,
			})
		}

		parityIndex := float64(0)
		if summary.Total > 0 {
			parityIndex = math.Round(summary.MatchRatePct*100) / 100
		}

		resp := auditTrailResponse{
			Status:             "populated",
			HasRun:             true,
			ReconciledVolume:   reconciledVolume,
			ParityIndex:        parityIndex,
			ReconciliationRate: reconciliationRate,
			ExactCount:         summary.Exact,
			FuzzyCount:         summary.Fuzzy,
			BatchCount:         summary.Batch,
			AICount:            aiResolvedCount,
			DeterministicCount: deterministicCount,
			UnresolvedCount:    summary.Exceptions,
			ReconciledCount:    reconciledCount,
			TotalCount:         summary.Total,
			Rows:               rows,
			Evaluation:         loadEvaluation(dataDir, results),
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ── POST /api/reset and DELETE /api/reset ────────────────────────────────

func handleReset(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filesToRemove := []string{
			"match_results_final.json",
			"match_results.json",
			"metrics.json",
			"resolutions.json",
			"report.html",
		}

		for _, f := range filesToRemove {
			p := filepath.Join(dataDir, f)
			if err := os.Remove(p); err == nil {
				log.Printf("reset: removed %s", f)
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "idle",
			"message": "Reconciliation state reset to idle. Match results cleared.",
		})
	}
}

// ── GET /api/sample/* (Downloadable template files) ──────────────────────

func handleSample(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format := strings.ToLower(r.URL.Query().Get("format"))
		if format != "json" {
			format = "csv"
		}

		ds := generator.Generate(10, 42)

		switch kind {
		case "settlements":
			filename := fmt.Sprintf("sample_settlements.%s", format)
			if format == "json" {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
				json.NewEncoder(w).Encode(ds.Settlements)
				return
			}
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
			cw := csv.NewWriter(w)
			cw.Write(settlementCSVHeaders)
			for _, s := range ds.Settlements {
				cw.Write([]string{
					s.SettlementID,
					s.PaymentID,
					s.OrderID,
					strconv.FormatInt(s.GrossAmountPaisa, 10),
					strconv.FormatInt(s.FeePaisa, 10),
					strconv.FormatInt(s.TaxPaisa, 10),
					strconv.FormatInt(s.NetAmountPaisa, 10),
					s.SettledAt.Format(time.RFC3339),
					string(s.Status),
				})
			}
			cw.Flush()

		case "bank-statements":
			filename := fmt.Sprintf("sample_bank_statements.%s", format)
			if format == "json" {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
				json.NewEncoder(w).Encode(ds.BankStatements)
				return
			}
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
			cw := csv.NewWriter(w)
			cw.Write(bankCSVHeaders)
			for _, b := range ds.BankStatements {
				cw.Write([]string{
					b.UTRRef,
					b.Narration,
					fmt.Sprintf("%.2f", b.CreditAmountINR),
					b.ValueDate.Format(time.RFC3339),
					strconv.FormatBool(b.BatchFlag),
					b.BatchID,
				})
			}
			cw.Flush()

		case "ledger":
			filename := fmt.Sprintf("sample_ledger_entries.%s", format)
			if format == "json" {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
				json.NewEncoder(w).Encode(ds.LedgerEntries)
				return
			}
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
			cw := csv.NewWriter(w)
			cw.Write(ledgerCSVHeaders)
			for _, l := range ds.LedgerEntries {
				cw.Write([]string{
					l.OrderID,
					l.CustomerID,
					fmt.Sprintf("%.2f", l.GrossAmountINR),
					l.ExpectedSettlementDate.Format(time.RFC3339),
					l.InternalStatus,
				})
			}
			cw.Flush()

		default:
			writeError(w, http.StatusBadRequest, "unknown sample kind")
		}
	}
}

// ── Auto-format detection helper ────────────────────────────────────────

func isCSVFormat(filename, contentType string, data []byte) bool {
	lowerName := strings.ToLower(filename)
	if strings.HasSuffix(lowerName, ".csv") {
		return true
	}
	if strings.HasSuffix(lowerName, ".json") {
		return false
	}
	lowerContent := strings.ToLower(contentType)
	if strings.Contains(lowerContent, "csv") {
		return true
	}
	if strings.Contains(lowerContent, "json") {
		return false
	}
	// Fallback sniffing: JSON starts with '[' or '{'
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("[")) || bytes.HasPrefix(trimmed, []byte("{")) {
		return false
	}
	return true
}

// ── POST /api/upload/* ──────────────────────────────────────────────────

type uploadResponse struct {
	Rows            int                      `json:"rows"`
	Preview         []map[string]interface{} `json:"preview"`
	Warnings        []string                 `json:"warnings"`
	ExpectedHeaders []string                 `json:"expectedHeaders,omitempty"`
	FoundHeaders    []string                 `json:"foundHeaders,omitempty"`
}

func handleUpload(dataDir, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse up to 32MB.
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("parsing multipart form: %v", err))
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("reading uploaded file: %v", err))
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("reading file content: %v", err))
			return
		}

		contentType := header.Header.Get("Content-Type")
		isCSV := isCSVFormat(header.Filename, contentType, data)

		var resp uploadResponse

		switch kind {
		case "settlements":
			resp, err = parseAndWriteSettlements(dataDir, data, isCSV)
		case "bank-statements":
			resp, err = parseAndWriteBankStatements(dataDir, data, isCSV)
		case "ledger":
			resp, err = parseAndWriteLedger(dataDir, data, isCSV)
		default:
			writeError(w, http.StatusBadRequest, "unknown upload kind")
			return
		}

		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("parsing %s: %v", kind, err))
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ── Settlement upload parsing ───────────────────────────────────────────

var settlementCSVHeaders = []string{
	"settlement_id", "payment_id", "order_id",
	"gross_amount_paisa", "fee_paisa", "tax_paisa",
	"net_amount_paisa", "settled_at", "status",
}

func parseAndWriteSettlements(dataDir string, data []byte, isCSV bool) (uploadResponse, error) {
	var settlements []model.Settlement
	var warnings []string
	var foundHeaders []string

	if isCSV {
		reader := csv.NewReader(strings.NewReader(string(data)))
		reader.FieldsPerRecord = -1
		reader.LazyQuotes = true
		records, err := reader.ReadAll()
		if err != nil {
			return uploadResponse{ExpectedHeaders: settlementCSVHeaders}, fmt.Errorf("CSV formatting error: %w", err)
		}

		if len(records) < 2 {
			return uploadResponse{ExpectedHeaders: settlementCSVHeaders}, fmt.Errorf("CSV must have a header row and at least one data row")
		}

		headers := records[0]
		foundHeaders = make([]string, len(headers))
		headerMap := make(map[string]int)
		for i, h := range headers {
			cleanH := strings.TrimSpace(strings.ToLower(h))
			foundHeaders[i] = cleanH
			headerMap[cleanH] = i
		}

		var missing []string
		for _, expected := range settlementCSVHeaders {
			if _, ok := headerMap[expected]; !ok {
				missing = append(missing, expected)
			}
		}
		if len(missing) > 0 {
			return uploadResponse{
				ExpectedHeaders: settlementCSVHeaders,
				FoundHeaders:    foundHeaders,
			}, fmt.Errorf("missing required column(s): [%s]. Found columns: [%s]", strings.Join(missing, ", "), strings.Join(foundHeaders, ", "))
		}

		for rowIdx, row := range records[1:] {
			if len(row) < len(headers) {
				warnings = append(warnings, fmt.Sprintf("row %d: insufficient columns (%d expected %d), skipped", rowIdx+2, len(row), len(headers)))
				continue
			}

			gross, err := strconv.ParseInt(strings.TrimSpace(row[headerMap["gross_amount_paisa"]]), 10, 64)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("row %d: invalid gross_amount_paisa (%q): %v", rowIdx+2, row[headerMap["gross_amount_paisa"]], err))
				continue
			}

			fee, err := strconv.ParseInt(strings.TrimSpace(row[headerMap["fee_paisa"]]), 10, 64)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("row %d: invalid fee_paisa: %v", rowIdx+2, err))
				continue
			}

			tax, err := strconv.ParseInt(strings.TrimSpace(row[headerMap["tax_paisa"]]), 10, 64)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("row %d: invalid tax_paisa: %v", rowIdx+2, err))
				continue
			}

			net, err := strconv.ParseInt(strings.TrimSpace(row[headerMap["net_amount_paisa"]]), 10, 64)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("row %d: invalid net_amount_paisa: %v", rowIdx+2, err))
				continue
			}

			rawDate := strings.TrimSpace(row[headerMap["settled_at"]])
			settledAt, err := time.Parse(time.RFC3339, rawDate)
			if err != nil {
				// Also try date-only fallback
				if dOnly, dErr := time.Parse("2006-01-02", rawDate); dErr == nil {
					settledAt = dOnly
				} else {
					warnings = append(warnings, fmt.Sprintf("row %d: invalid settled_at date format (%q, expected RFC3339 e.g. 2026-02-01T10:00:00Z)", rowIdx+2, rawDate))
					continue
				}
			}

			settlements = append(settlements, model.Settlement{
				SettlementID:     strings.TrimSpace(row[headerMap["settlement_id"]]),
				PaymentID:        strings.TrimSpace(row[headerMap["payment_id"]]),
				OrderID:          strings.TrimSpace(row[headerMap["order_id"]]),
				GrossAmountPaisa: gross,
				FeePaisa:         fee,
				TaxPaisa:         tax,
				NetAmountPaisa:   net,
				SettledAt:        settledAt,
				Status:           model.Settlement_Status(strings.TrimSpace(row[headerMap["status"]])),
			})
		}
	} else {
		if err := json.Unmarshal(data, &settlements); err != nil {
			return uploadResponse{}, fmt.Errorf("invalid JSON: %w (ensure valid JSON array of settlement objects)", err)
		}
	}

	if len(settlements) == 0 {
		return uploadResponse{Warnings: warnings, FoundHeaders: foundHeaders, ExpectedHeaders: settlementCSVHeaders},
			fmt.Errorf("no valid settlement rows parsed from input")
	}

	if err := writeJSONFile(filepath.Join(dataDir, "settlements.json"), settlements); err != nil {
		return uploadResponse{}, fmt.Errorf("saving file to disk: %w", err)
	}

	// Build 2-3 preview rows.
	var preview []map[string]interface{}
	for i := 0; i < len(settlements) && i < 3; i++ {
		s := settlements[i]
		preview = append(preview, map[string]interface{}{
			"order_id":      s.OrderID,
			"settlement_id": s.SettlementID,
			"payment_id":    s.PaymentID,
			"net_amount":    formatINR(float64(s.NetAmountPaisa) / 100),
			"settled_at":    s.SettledAt.Format("2006-01-02 15:04"),
			"status":        string(s.Status),
		})
	}

	return uploadResponse{
		Rows:            len(settlements),
		Preview:         preview,
		Warnings:        warnings,
		FoundHeaders:    foundHeaders,
		ExpectedHeaders: settlementCSVHeaders,
	}, nil
}

// ── Bank statement upload parsing ───────────────────────────────────────

var bankCSVHeaders = []string{
	"utr_ref", "narration", "credit_amount_inr",
	"value_date", "batch_flag", "batch_id",
}

var bankRequiredCSVHeaders = []string{
	"utr_ref", "narration", "credit_amount_inr",
	"value_date", "batch_flag",
}

func parseAndWriteBankStatements(dataDir string, data []byte, isCSV bool) (uploadResponse, error) {
	var statements []model.BankStatement
	var warnings []string
	var foundHeaders []string

	if isCSV {
		reader := csv.NewReader(strings.NewReader(string(data)))
		reader.FieldsPerRecord = -1
		reader.LazyQuotes = true
		records, err := reader.ReadAll()
		if err != nil {
			return uploadResponse{ExpectedHeaders: bankRequiredCSVHeaders}, fmt.Errorf("CSV formatting error: %w", err)
		}

		if len(records) < 2 {
			return uploadResponse{ExpectedHeaders: bankRequiredCSVHeaders}, fmt.Errorf("CSV must have a header row and at least one data row")
		}

		headers := records[0]
		foundHeaders = make([]string, len(headers))
		headerMap := make(map[string]int)
		for i, h := range headers {
			cleanH := strings.TrimSpace(strings.ToLower(h))
			foundHeaders[i] = cleanH
			headerMap[cleanH] = i
		}

		var missing []string
		for _, required := range bankRequiredCSVHeaders {
			if _, ok := headerMap[required]; !ok {
				missing = append(missing, required)
			}
		}
		if len(missing) > 0 {
			return uploadResponse{
				ExpectedHeaders: bankRequiredCSVHeaders,
				FoundHeaders:    foundHeaders,
			}, fmt.Errorf("missing required column(s): [%s]. Found columns: [%s]", strings.Join(missing, ", "), strings.Join(foundHeaders, ", "))
		}

		for rowIdx, row := range records[1:] {
			if len(row) < len(headers) {
				warnings = append(warnings, fmt.Sprintf("row %d: insufficient columns (%d expected %d), skipped", rowIdx+2, len(row), len(headers)))
				continue
			}

			creditAmt, err := strconv.ParseFloat(strings.TrimSpace(row[headerMap["credit_amount_inr"]]), 64)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("row %d: invalid credit_amount_inr (%q): %v", rowIdx+2, row[headerMap["credit_amount_inr"]], err))
				continue
			}

			rawDate := strings.TrimSpace(row[headerMap["value_date"]])
			valueDate, err := time.Parse(time.RFC3339, rawDate)
			if err != nil {
				if dOnly, dErr := time.Parse("2006-01-02", rawDate); dErr == nil {
					valueDate = dOnly
				} else {
					warnings = append(warnings, fmt.Sprintf("row %d: invalid value_date format (%q, expected RFC3339)", rowIdx+2, rawDate))
					continue
				}
			}

			batchFlag := strings.TrimSpace(strings.ToLower(row[headerMap["batch_flag"]])) == "true"

			bs := model.BankStatement{
				UTRRef:          strings.TrimSpace(row[headerMap["utr_ref"]]),
				Narration:       strings.TrimSpace(row[headerMap["narration"]]),
				CreditAmountINR: creditAmt,
				ValueDate:       valueDate,
				BatchFlag:       batchFlag,
			}

			if idx, ok := headerMap["batch_id"]; ok && idx < len(row) {
				bs.BatchID = strings.TrimSpace(row[idx])
			}

			statements = append(statements, bs)
		}
	} else {
		if err := json.Unmarshal(data, &statements); err != nil {
			return uploadResponse{}, fmt.Errorf("invalid JSON: %w (ensure valid JSON array of bank statement objects)", err)
		}
	}

	if len(statements) == 0 {
		return uploadResponse{Warnings: warnings, FoundHeaders: foundHeaders, ExpectedHeaders: bankCSVHeaders},
			fmt.Errorf("no valid bank statement rows parsed from input")
	}

	if err := writeJSONFile(filepath.Join(dataDir, "bank_statements.json"), statements); err != nil {
		return uploadResponse{}, fmt.Errorf("saving file to disk: %w", err)
	}

	// Build 2-3 preview rows.
	var preview []map[string]interface{}
	for i := 0; i < len(statements) && i < 3; i++ {
		b := statements[i]
		preview = append(preview, map[string]interface{}{
			"utr_ref":       b.UTRRef,
			"narration":     b.Narration,
			"credit_amount": formatINR(b.CreditAmountINR),
			"value_date":    b.ValueDate.Format("2006-01-02"),
			"batch_flag":    b.BatchFlag,
		})
	}

	return uploadResponse{
		Rows:            len(statements),
		Preview:         preview,
		Warnings:        warnings,
		FoundHeaders:    foundHeaders,
		ExpectedHeaders: bankCSVHeaders,
	}, nil
}

// ── Ledger upload parsing ───────────────────────────────────────────────

var ledgerCSVHeaders = []string{
	"order_id", "customer_id", "gross_amount_inr",
	"expected_settlement_date", "internal_status",
}

func parseAndWriteLedger(dataDir string, data []byte, isCSV bool) (uploadResponse, error) {
	var entries []model.LedgerEntry
	var warnings []string
	var foundHeaders []string

	if isCSV {
		reader := csv.NewReader(strings.NewReader(string(data)))
		reader.FieldsPerRecord = -1
		reader.LazyQuotes = true
		records, err := reader.ReadAll()
		if err != nil {
			return uploadResponse{ExpectedHeaders: ledgerCSVHeaders}, fmt.Errorf("CSV formatting error: %w", err)
		}

		if len(records) < 2 {
			return uploadResponse{ExpectedHeaders: ledgerCSVHeaders}, fmt.Errorf("CSV must have a header row and at least one data row")
		}

		headers := records[0]
		foundHeaders = make([]string, len(headers))
		headerMap := make(map[string]int)
		for i, h := range headers {
			cleanH := strings.TrimSpace(strings.ToLower(h))
			foundHeaders[i] = cleanH
			headerMap[cleanH] = i
		}

		var missing []string
		for _, expected := range ledgerCSVHeaders {
			if _, ok := headerMap[expected]; !ok {
				missing = append(missing, expected)
			}
		}
		if len(missing) > 0 {
			return uploadResponse{
				ExpectedHeaders: ledgerCSVHeaders,
				FoundHeaders:    foundHeaders,
			}, fmt.Errorf("missing required column(s): [%s]. Found columns: [%s]", strings.Join(missing, ", "), strings.Join(foundHeaders, ", "))
		}

		for rowIdx, row := range records[1:] {
			if len(row) < len(headers) {
				warnings = append(warnings, fmt.Sprintf("row %d: insufficient columns (%d expected %d), skipped", rowIdx+2, len(row), len(headers)))
				continue
			}

			grossAmt, err := strconv.ParseFloat(strings.TrimSpace(row[headerMap["gross_amount_inr"]]), 64)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("row %d: invalid gross_amount_inr (%q): %v", rowIdx+2, row[headerMap["gross_amount_inr"]], err))
				continue
			}

			rawDate := strings.TrimSpace(row[headerMap["expected_settlement_date"]])
			expectedDate, err := time.Parse(time.RFC3339, rawDate)
			if err != nil {
				if dOnly, dErr := time.Parse("2006-01-02", rawDate); dErr == nil {
					expectedDate = dOnly
				} else {
					warnings = append(warnings, fmt.Sprintf("row %d: invalid expected_settlement_date format (%q, expected RFC3339)", rowIdx+2, rawDate))
					continue
				}
			}

			entries = append(entries, model.LedgerEntry{
				OrderID:                strings.TrimSpace(row[headerMap["order_id"]]),
				CustomerID:             strings.TrimSpace(row[headerMap["customer_id"]]),
				GrossAmountINR:         grossAmt,
				ExpectedSettlementDate: expectedDate,
				InternalStatus:         strings.TrimSpace(row[headerMap["internal_status"]]),
			})
		}
	} else {
		if err := json.Unmarshal(data, &entries); err != nil {
			return uploadResponse{}, fmt.Errorf("invalid JSON: %w (ensure valid JSON array of ledger objects)", err)
		}
	}

	if len(entries) == 0 {
		return uploadResponse{Warnings: warnings, FoundHeaders: foundHeaders, ExpectedHeaders: ledgerCSVHeaders},
			fmt.Errorf("no valid ledger rows parsed from input")
	}

	if err := writeJSONFile(filepath.Join(dataDir, "ledger_entries.json"), entries); err != nil {
		return uploadResponse{}, fmt.Errorf("saving file to disk: %w", err)
	}

	// Build 2-3 preview rows.
	var preview []map[string]interface{}
	for i := 0; i < len(entries) && i < 3; i++ {
		l := entries[i]
		preview = append(preview, map[string]interface{}{
			"order_id":        l.OrderID,
			"customer_id":     l.CustomerID,
			"gross_amount":    formatINR(l.GrossAmountINR),
			"internal_status": l.InternalStatus,
			"expected_date":   l.ExpectedSettlementDate.Format("2006-01-02"),
		})
	}

	return uploadResponse{
		Rows:            len(entries),
		Preview:         preview,
		Warnings:        warnings,
		FoundHeaders:    foundHeaders,
		ExpectedHeaders: ledgerCSVHeaders,
	}, nil
}

// ── POST /api/run ───────────────────────────────────────────────────────

type runResponse struct {
	Summary    overviewResponse `json:"summary"`
	LLMSkipped bool             `json:"llmSkipped"`
	Message    string           `json:"message"`
}

func handleRun(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Step 1: Load data.
		d, err := loader.Load(dataDir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("loading data: %v (make sure settlements, bank statements, and ledger are all uploaded)", err))
			return
		}

		// Step 2: Run matcher.
		start := time.Now()
		results := matcher.Match(d.Settlements, d.BankStatements, d.LedgerEntries)
		elapsed := time.Since(start)

		processingTimeMs := float64(elapsed.Nanoseconds()) / 1e6
		throughput := float64(0)
		if processingTimeMs > 0 {
			throughput = float64(len(d.Settlements)) / (processingTimeMs / 1000)
		}

		metrics := map[string]float64{
			"processing_time_ms": processingTimeMs,
			"records_per_second": throughput,
		}

		writeJSONFile(filepath.Join(dataDir, "metrics.json"), metrics)
		writeJSONFile(filepath.Join(dataDir, "match_results.json"), results)

		log.Printf("match complete: %d results in %.3fms", len(results), processingTimeMs)

		// Step 3: Optionally run resolver.
		llmSkipped := true
		message := "Match complete."

		apiKey := os.Getenv("GROQ_API_KEY")
		if apiKey != "" {
			llmSkipped = false
			message = "Match + LLM resolve complete."

			settlementLookup := make(map[string]int)
			for i, s := range d.Settlements {
				settlementLookup[s.SettlementID] = i
			}

			// Remove stale final results.
			finalPath := filepath.Join(dataDir, "match_results_final.json")
			os.Remove(finalPath)

			candidatePool := resolver.UnusedBankStatements(d.BankStatements, results)
			client := resolver.NewClient(apiKey, "openai/gpt-oss-120b")

			var resolutions []resolver.Resolution
			upgraded := 0
			failed := 0

			for i, res := range results {
				if res.Tier != matcher.TierUnresolved {
					continue
				}

				sIdx, ok := settlementLookup[res.SettlementID]
				if !ok {
					continue
				}

				resolution, err := client.Resolve(d.Settlements[sIdx], candidatePool)
				if err != nil {
					log.Printf("WARN: resolving %s failed: %v", res.SettlementID, err)
					failed++
					continue
				}

				resolutions = append(resolutions, resolution)

				if resolution.Decision == "MATCH" {
					upgraded++
					results[i].Tier = "llm_resolved"
					results[i].BankUTRRef = resolution.BankUTRRef
					results[i].Reason = fmt.Sprintf("[LLM confidence %.2f] %s", resolution.Confidence, resolution.Reason)

					for j, c := range candidatePool {
						if c.UTRRef == resolution.BankUTRRef {
							candidatePool = append(candidatePool[:j], candidatePool[j+1:]...)
							break
						}
					}
				} else {
					results[i].Reason = fmt.Sprintf("confirmed exception: %s", resolution.Reason)
				}
			}

			writeJSONFile(filepath.Join(dataDir, "resolutions.json"), resolutions)
			writeJSONFile(finalPath, results)

			if failed > 0 {
				message = fmt.Sprintf("Match + LLM resolve complete (%d resolve calls failed).", failed)
			}
		} else {
			// No LLM key - write match results as final.
			writeJSONFile(filepath.Join(dataDir, "match_results_final.json"), results)
			message = "Match complete. LLM resolution skipped (GROQ_API_KEY not set)."
		}

		// Step 4: Build overview summary for immediate frontend consumption.
		summary := report.Summarize(results)
		deterministicCount := summary.Exact + summary.Fuzzy + summary.Batch
		aiResolvedCount := summary.LLMResolved
		matchedCount := deterministicCount + aiResolvedCount
		total := summary.Total
		reconciliationRate := math.Round(summary.MatchRatePct*10) / 10

		pct := func(count int) float64 {
			if total == 0 {
				return 0
			}
			return math.Round(float64(count)/float64(total)*1000) / 10
		}

		var totalNetPaisa int64
		for _, s := range d.Settlements {
			totalNetPaisa += s.NetAmountPaisa
		}

		avgResTime := "< 1µs / tx"
		if processingTimeMs > 0 && total > 0 {
			avgMs := processingTimeMs / float64(total)
			if avgMs < 0.01 {
				avgMicroseconds := avgMs * 1000
				avgResTime = fmt.Sprintf("%.1fµs / tx", avgMicroseconds)
			} else if avgMs < 1 {
				avgResTime = fmt.Sprintf("%.2fms / tx", avgMs)
			} else {
				avgResTime = fmt.Sprintf("%.1fms / tx", avgMs)
			}
		}

		resp := runResponse{
			Summary: overviewResponse{
				Status:             "populated",
				HasRun:             true,
				LastUpdated:        time.Now().Format("Today at 3:04 PM"),
				SyncDescription:    "Pipeline run completed",
				ReconciliationRate: reconciliationRate,
				Velocity:           reconciliationRate,
				MatchedCount:       matchedCount,
				DeterministicCount: deterministicCount,
				AIResolvedCount:    aiResolvedCount,
				TotalCount:         total,
				NeedReview:         summary.Exceptions,
				ProcessedVolume:    formatINR(float64(totalNetPaisa) / 100),
				BatchID:            "#LIVE-DATA",
				Tiers: overviewTiers{
					Exact:      tierInfo{Label: "EXACT MATCH", Count: summary.Exact, Pct: pct(summary.Exact), Color: "green"},
					Fuzzy:      tierInfo{Label: "FUZZY MATCH", Count: summary.Fuzzy, Pct: pct(summary.Fuzzy), Color: "lavender"},
					Batch:      tierInfo{Label: "BATCH RECONCILIATION", Count: summary.Batch, Pct: pct(summary.Batch), Color: "blue"},
					AIResolved: tierInfo{Label: "AI-RESOLVED", Count: aiResolvedCount, Pct: pct(aiResolvedCount), Color: "purple"},
					Unresolved: tierInfo{Label: "UNRESOLVED (EXCEPTIONS)", Count: summary.Exceptions, Pct: pct(summary.Exceptions), Color: "red"},
				},
				Stats: overviewStats{
					TotalTransactions:  total,
					AvgResolutionTime:  avgResTime,
					ExceptionsThisWeek: summary.Exceptions,
				},
				Evaluation: loadEvaluation(dataDir, results),
			},
			LLMSkipped: llmSkipped,
			Message:    message,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────

func mapTier(tier matcher.Tier) (string, string) {
	switch tier {
	case matcher.TierExact:
		return "EXACT", "green"
	case matcher.TierFuzzy:
		return "FUZZY", "lavender"
	case matcher.TierBatch:
		return "BATCH", "blue"
	case "llm_resolved":
		return "AI-RESOLVED", "purple"
	case matcher.TierUnresolved:
		return "UNRESOLVED", "red"
	default:
		return "UNRESOLVED", "red"
	}
}

// formatINR formats a float as Indian Rupee with the ₹ prefix and commas
// in the Indian numbering system (e.g. ₹14,82,490.00).
func formatINR(amount float64) string {
	negative := amount < 0
	amount = math.Abs(amount)

	whole := int64(amount)
	frac := int64(math.Round((amount - float64(whole)) * 100))

	if frac >= 100 {
		whole++
		frac -= 100
	}

	s := fmt.Sprintf("%d", whole)
	if len(s) > 3 {
		result := s[len(s)-3:]
		s = s[:len(s)-3]
		for len(s) > 2 {
			result = s[len(s)-2:] + "," + result
			s = s[:len(s)-2]
		}
		if len(s) > 0 {
			result = s + "," + result
		}
		s = result
	}

	prefix := "₹"
	if negative {
		prefix = "-₹"
	}

	return fmt.Sprintf("%s%s.%02d", prefix, s, frac)
}

func formatINRSigned(amount float64) string {
	if amount >= 0 {
		return "+" + formatINR(amount)
	}
	return formatINR(amount)
}
