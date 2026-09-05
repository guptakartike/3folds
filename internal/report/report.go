// Package report aggregates final match results into the numbers the
// track's bar explicitly asks for (match rate, honest exception list)
// and renders role-specific views: a founder summary, a finance-exec
// exception queue, and a full audit trail.
package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"threefolds/internal/matcher"
)

// Summary is the aggregate numbers computed across all final results.
type Summary struct {
	Total                 int
	Exact                 int
	Fuzzy                 int
	Batch                 int
	LLMResolved           int
	LLMReviewed           int
	ConfirmedExceptions   int
	Exceptions            int
	ExpectedMatches       int
	ExpectedExceptions    int
	DetectedExceptions    int
	Matched               int
	MatchRatePct          float64
	ClassificationPct     float64
	ExceptionDetectionPct float64
	FalsePositives        int
	FalseNegatives        int
	ProcessingTimeMs      float64
	RecordsPerSecond      float64
}

// Load reads match_results_final.json if it exists (i.e. resolve was
// run), otherwise falls back to match_results.json.
func Load(dir string) ([]matcher.Result, string, error) {
	finalPath := filepath.Join(dir, "match_results_final.json")
	if _, err := os.Stat(finalPath); err == nil {
		var results []matcher.Result
		if err := readJSON(finalPath, &results); err != nil {
			return nil, "", err
		}
		return results, finalPath, nil
	}

	basicPath := filepath.Join(dir, "match_results.json")
	var results []matcher.Result
	if err := readJSON(basicPath, &results); err != nil {
		return nil, "", err
	}

	return results, basicPath, nil
}

func readJSON(path string, v interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewDecoder(f).Decode(v)
}

// Summarize computes the aggregate numbers from a result set.
func Summarize(results []matcher.Result) Summary {
	s := Summary{Total: len(results)}

	for _, r := range results {
		switch r.Tier {
		case matcher.TierExact:
			s.Exact++

		case matcher.TierFuzzy:
			s.Fuzzy++

		case matcher.TierBatch:
			s.Batch++

		case "llm_resolved":
			s.LLMResolved++

		case matcher.TierUnresolved:
			s.Exceptions++

		default:
			s.Exceptions++
		}
	}

	if s.Total > 0 {
		resolved := s.Exact + s.Fuzzy + s.Batch + s.LLMResolved
		s.MatchRatePct = float64(resolved) / float64(s.Total) * 100
	}

	return s
}

// SetThroughput adds measured reconciliation performance to the summary.
func SetThroughput(s Summary, processingTimeMs float64) Summary {
	s.ProcessingTimeMs = processingTimeMs

	if processingTimeMs > 0 {
		s.RecordsPerSecond =
			float64(s.Total) / (processingTimeMs / 1000)
	}

	return s
}

// SetEvaluation adds ground-truth evaluation metrics to the report.
func SetEvaluation(
	s Summary,
	total, correct, matched, expectedMatches, expectedExceptions,
	detectedExceptions, falsePositives, falseNegatives int,
) Summary {
	s.ClassificationPct = 0
	if total > 0 {
		s.ClassificationPct = float64(correct) / float64(total) * 100
	}

	s.Matched = matched
	s.ExpectedMatches = expectedMatches
	s.ExpectedExceptions = expectedExceptions
	s.DetectedExceptions = detectedExceptions
	s.FalsePositives = falsePositives
	s.FalseNegatives = falseNegatives

	if expectedExceptions > 0 {
		s.ExceptionDetectionPct =
			float64(detectedExceptions) / float64(expectedExceptions) * 100
	} else {
		s.ExceptionDetectionPct = 100
	}

	return s
}

// SetLLM records how many unresolved cases were reviewed by the LLM and
// how many of those were confirmed as exceptions.
func SetLLM(s Summary, reviewed, confirmedExceptions int) Summary {
	s.LLMReviewed = reviewed
	s.ConfirmedExceptions = confirmedExceptions
	return s
}

// Exceptions returns only the still-unresolved results.
func Exceptions(results []matcher.Result) []matcher.Result {
	var out []matcher.Result

	for _, r := range results {
		if r.Tier == matcher.TierUnresolved {
			out = append(out, r)
		}
	}

	return out
}

// PrintCLI writes a plain-text summary to stdout.
func PrintCLI(s Summary, exceptions []matcher.Result) {
	fmt.Printf("\n=== 3folds final reconciliation report ===\n")
	fmt.Printf("total settlements:        %d\n", s.Total)
	fmt.Printf("automatic match rate:     %.1f%%\n", s.MatchRatePct)
	fmt.Printf("classification accuracy:  %.1f%%\n", s.ClassificationPct)
	fmt.Printf("  exact:                  %d\n", s.Exact)
	fmt.Printf("  fuzzy:                  %d\n", s.Fuzzy)
	fmt.Printf("  batch:                  %d\n", s.Batch)
	fmt.Printf("  llm-resolved:           %d\n", s.LLMResolved)
	fmt.Printf("  exceptions:             %d\n", s.Exceptions)

	if s.ExpectedMatches > 0 {
		fmt.Printf("\nexpected matches:         %d\n", s.ExpectedMatches)
		fmt.Printf("matched:                  %d\n", s.Matched)
		fmt.Printf("match coverage:           %.1f%%\n", float64(s.Matched)/float64(s.ExpectedMatches)*100)
	}
	if s.ExpectedExceptions > 0 {
		fmt.Printf("\nexpected exceptions:      %d\n", s.ExpectedExceptions)
		fmt.Printf("detected exceptions:      %d\n", s.DetectedExceptions)
		fmt.Printf("exception detection:      %.1f%%\n", s.ExceptionDetectionPct)
	}
	fmt.Printf("false positives:           %d\n", s.FalsePositives)
	fmt.Printf("false negatives:           %d\n", s.FalseNegatives)

	if s.LLMReviewed > 0 {
		fmt.Printf("\nllm reviewed:             %d\n", s.LLMReviewed)
		fmt.Printf("llm-confirmed exceptions: %d\n", s.ConfirmedExceptions)
	}

	if s.ProcessingTimeMs > 0 {
		fmt.Printf("\nprocessing time:          %.3f ms\n", s.ProcessingTimeMs)
		fmt.Printf("throughput:               %.0f records/sec\n", s.RecordsPerSecond)
	}

	if len(exceptions) > 0 {
		fmt.Printf("\n--- honest exception list (unresolved) ---\n")

		for _, e := range exceptions {
			fmt.Printf(
				"  order=%s settlement=%s reason=%q\n",
				e.OrderID,
				e.SettlementID,
				e.Reason,
			)
		}
	}
}

// roleData is what the HTML template renders.
type roleData struct {
	Summary    Summary
	Results    []matcher.Result
	Exceptions []matcher.Result
}

const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>3folds — Reconciliation Report</title>

<style>
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    max-width: 1100px;
    margin: 40px auto;
    padding: 0 20px;
    color: #1a1a1a;
    background: #ffffff;
  }

  h1 {
    font-size: 26px;
    margin-bottom: 6px;
  }

  h2 {
    font-size: 17px;
    margin-top: 36px;
    border-bottom: 1px solid #ddd;
    padding-bottom: 8px;
  }

  .subtitle {
    color: #666;
    margin-bottom: 28px;
  }

  .founder-box {
    background: #f4f4f4;
    border-radius: 10px;
    padding: 24px;
    margin: 16px 0;
  }

  .rate {
    font-size: 38px;
    font-weight: 700;
    margin-bottom: 4px;
  }

  .accuracy {
    font-size: 16px;
    font-weight: 600;
    margin-bottom: 4px;
  }

  .subtle {
    color: #666;
    font-size: 13px;
  }

  .metrics {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 12px;
    margin-top: 20px;
  }

  .metric {
    border: 1px solid #ddd;
    border-radius: 8px;
    padding: 16px;
  }

  .metric-value {
    font-size: 22px;
    font-weight: 700;
  }

  .metric-label {
    color: #666;
    font-size: 12px;
    margin-top: 4px;
  }

  table {
    border-collapse: collapse;
    width: 100%;
    margin-top: 12px;
    font-size: 13px;
  }

  th, td {
    text-align: left;
    padding: 8px 10px;
    border-bottom: 1px solid #eee;
    vertical-align: top;
  }

  th {
    background: #fafafa;
  }

  .tier-exact {
    color: #1a7f37;
    font-weight: 600;
  }

  .tier-fuzzy {
    color: #9a6700;
    font-weight: 600;
  }

  .tier-batch {
    color: #0969da;
    font-weight: 600;
  }

  .tier-llm_resolved {
    color: #6639ba;
    font-weight: 600;
  }

  .tier-unresolved {
    color: #cf222e;
    font-weight: 600;
  }

  .exception {
    background: #fff8f8;
  }

  @media (max-width: 700px) {
    .metrics {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>
</head>

<body>

<h1>3folds — Multi-Source Reconciliation Report</h1>
<div class="subtitle">
  Deterministic reconciliation with measured accuracy, throughput, and honest exceptions.
</div>

<h2>Founder summary</h2>

<div class="founder-box">

  <div class="rate">
    {{printf "%.1f" .Summary.MatchRatePct}}% reconciled
  </div>

  <div class="accuracy">
    {{printf "%.1f" .Summary.ClassificationPct}}% match / exception classification accuracy
  </div>

  <div class="subtle">
    {{.Summary.Exceptions}} of {{.Summary.Total}} transactions remain open exceptions.
  </div>

  <div class="metrics">

    <div class="metric">
      <div class="metric-value">{{.Summary.Total}}</div>
      <div class="metric-label">Total settlements</div>
    </div>

    <div class="metric">
      <div class="metric-value">{{.Summary.Exact}}</div>
      <div class="metric-label">Exact matches</div>
    </div>

    <div class="metric">
      <div class="metric-value">{{.Summary.Fuzzy}}</div>
      <div class="metric-label">Fuzzy matches</div>
    </div>

    <div class="metric">
      <div class="metric-value">{{.Summary.Batch}}</div>
      <div class="metric-label">Batch matches</div>
    </div>

    <div class="metric">
      <div class="metric-value">{{.Summary.LLMReviewed}}</div>
      <div class="metric-label">LLM reviewed</div>
    </div>

    <div class="metric">
      <div class="metric-value">{{.Summary.ConfirmedExceptions}}</div>
      <div class="metric-label">Exceptions confirmed</div>
    </div>

    {{if .Summary.ProcessingTimeMs}}
    <div class="metric">
      <div class="metric-value">
        {{printf "%.3f" .Summary.ProcessingTimeMs}} ms
      </div>
      <div class="metric-label">Processing time</div>
    </div>

    <div class="metric">
      <div class="metric-value">
        {{printf "%.0f" .Summary.RecordsPerSecond}}
      </div>
      <div class="metric-label">Records / second</div>
    </div>
    {{end}}

  </div>
</div>

<h2>Finance exec — exception queue</h2>

<table>
  <tr>
    <th>Order ID</th>
    <th>Settlement ID</th>
    <th>Reason</th>
  </tr>

  {{range .Exceptions}}
  <tr class="exception">
    <td>{{.OrderID}}</td>
    <td>{{.SettlementID}}</td>
    <td>{{.Reason}}</td>
  </tr>
  {{else}}
  <tr>
    <td colspan="3">No open exceptions.</td>
  </tr>
  {{end}}
</table>

<h2>Reconciliation breakdown</h2>

<table>
  <tr>
    <th>Tier</th>
    <th>Count</th>
    <th>Meaning</th>
  </tr>

  <tr>
    <td class="tier-exact">Exact</td>
    <td>{{.Summary.Exact}}</td>
    <td>Payment ID, amount, and timing aligned.</td>
  </tr>

  <tr>
    <td class="tier-fuzzy">Fuzzy</td>
    <td>{{.Summary.Fuzzy}}</td>
    <td>Minor amount or timing difference resolved.</td>
  </tr>

  <tr>
    <td class="tier-batch">Batch</td>
    <td>{{.Summary.Batch}}</td>
    <td>Multiple settlements reconciled against one bank batch.</td>
  </tr>

  <tr>
    <td class="tier-llm_resolved">LLM resolved</td>
    <td>{{.Summary.LLMResolved}}</td>
    <td>Previously unresolved case resolved with additional evidence and reasoning.</td>
  </tr>

  <tr>
    <td class="tier-unresolved">Exception</td>
    <td>{{.Summary.Exceptions}}</td>
    <td>No sufficient evidence to reconcile; safely escalated rather than force-matched.</td>
  </tr>
</table>

<h2>Evaluation</h2>

<table>
  <tr><th>Metric</th><th>Result</th></tr>
  <tr><td>Classification accuracy</td><td>{{printf "%.1f" .Summary.ClassificationPct}}%</td></tr>
  <tr><td>Expected matches found</td><td>{{.Summary.Matched}} / {{.Summary.ExpectedMatches}}</td></tr>
  <tr><td>False positives</td><td>{{.Summary.FalsePositives}}</td></tr>
  <tr><td>False negatives</td><td>{{.Summary.FalseNegatives}}</td></tr>
  <tr><td>Exception detection</td><td>{{.Summary.DetectedExceptions}} / {{.Summary.ExpectedExceptions}} ({{printf "%.1f" .Summary.ExceptionDetectionPct}}%)</td></tr>
</table>

<h2>Full audit trail</h2>

<table>
  <tr>
    <th>Order ID</th>
    <th>Settlement ID</th>
    <th>Tier</th>
    <th>Bank UTR</th>
    <th>Reason</th>
  </tr>

  {{range .Results}}
  <tr>
    <td>{{.OrderID}}</td>
    <td>{{.SettlementID}}</td>
    <td class="tier-{{.Tier}}">{{.Tier}}</td>
    <td>{{.BankUTRRef}}</td>
    <td>{{.Reason}}</td>
  </tr>
  {{end}}
</table>

</body>
</html>`

// WriteHTML renders the role-based report to path.
func WriteHTML(
	path string,
	s Summary,
	results []matcher.Result,
	exceptions []matcher.Result,
) error {
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(
		f,
		roleData{
			Summary:    s,
			Results:    results,
			Exceptions: exceptions,
		},
	)
}
