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

	"threefolds/internal/matcher"
)

// Summary is the aggregate numbers computed across all final results.
type Summary struct {
	Total        int
	Exact        int
	Fuzzy        int
	Batch        int
	LLMResolved  int
	Exceptions   int
	MatchRatePct float64 // (Exact+Fuzzy+LLMResolved) / Total * 100
}

// Load reads match_results_final.json if it exists (i.e. resolve was
// run), otherwise falls back to match_results.json (deterministic
// tiers only).
func Load(dir string) ([]matcher.Result, string, error) {
	finalPath := dir + "/match_results_final.json"
	if _, err := os.Stat(finalPath); err == nil {
		var results []matcher.Result
		if err := readJSON(finalPath, &results); err != nil {
			return nil, "", err
		}
		return results, finalPath, nil
	}

	basicPath := dir + "/match_results.json"
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

// Summarize computes the aggregate numbers from a result set. Any tier
// other than "unresolved" counts as resolved for the match rate,
// matching the track's "measured accuracy" requirement.
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

// Exceptions returns only the still-unresolved results, i.e. the
// "honest exception list" the track explicitly scores.
func Exceptions(results []matcher.Result) []matcher.Result {
	var out []matcher.Result
	for _, r := range results {
		if r.Tier == matcher.TierUnresolved {
			out = append(out, r)
		}
	}
	return out
}

// PrintCLI writes a plain-text summary to stdout — this is what you'd
// show live during a demo.
func PrintCLI(s Summary, exceptions []matcher.Result) {
	fmt.Printf("\n=== 3folds final reconciliation report ===\n")
	fmt.Printf("total settlements:  %d\n", s.Total)
	fmt.Printf("match rate:         %.1f%%\n", s.MatchRatePct)
	fmt.Printf("  exact:            %d\n", s.Exact)
	fmt.Printf("  fuzzy:            %d\n", s.Fuzzy)
	fmt.Printf("  batch:            %d\n", s.Batch)
	fmt.Printf("  llm-resolved:     %d\n", s.LLMResolved)
	fmt.Printf("  exceptions:       %d\n", s.Exceptions)

	if len(exceptions) > 0 {
		fmt.Printf("\n--- honest exception list (unresolved) ---\n")
		for _, e := range exceptions {
			fmt.Printf("  order=%s settlement=%s reason=%q\n", e.OrderID, e.SettlementID, e.Reason)
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
<title>3folds — reconciliation report</title>
<style>
  body { font-family: -apple-system, sans-serif; max-width: 900px; margin: 40px auto; padding: 0 20px; color: #1a1a1a; }
  h1 { font-size: 22px; } h2 { font-size: 17px; margin-top: 36px; border-bottom: 1px solid #ddd; padding-bottom: 6px; }
  .founder-box { background: #f4f4f4; border-radius: 8px; padding: 20px; margin: 16px 0; }
  .founder-box .rate { font-size: 32px; font-weight: 700; }
  table { border-collapse: collapse; width: 100%; margin-top: 12px; font-size: 13px; }
  th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid #eee; }
  th { background: #fafafa; }
  .tier-exact { color: #1a7f37; } .tier-fuzzy { color: #9a6700; }
  .tier-llm_resolved { color: #6639ba; } .tier-unresolved { color: #cf222e; font-weight: 600; }
</style>
</head>
<body>
  <h1>3folds — Multi-Source Reconciliation Report</h1>

  <h2>Founder summary</h2>
  <div class="founder-box">
    <div class="rate">{{printf "%.1f" .Summary.MatchRatePct}}% reconciled</div>
    <div>{{.Summary.Exceptions}} of {{.Summary.Total}} transactions still unresolved</div>
  </div>

  <h2>Finance exec — exception queue</h2>
  <table>
    <tr><th>Order ID</th><th>Settlement ID</th><th>Reason</th></tr>
    {{range .Exceptions}}
    <tr><td>{{.OrderID}}</td><td>{{.SettlementID}}</td><td>{{.Reason}}</td></tr>
    {{else}}
    <tr><td colspan="3">No open exceptions.</td></tr>
    {{end}}
  </table>

  <h2>Full audit trail</h2>
  <table>
    <tr><th>Order ID</th><th>Settlement ID</th><th>Tier</th><th>Bank UTR</th><th>Reason</th></tr>
    {{range .Results}}
    <tr>
      <td>{{.OrderID}}</td><td>{{.SettlementID}}</td>
      <td class="tier-{{.Tier}}">{{.Tier}}</td>
      <td>{{.BankUTRRef}}</td><td>{{.Reason}}</td>
    </tr>
    {{end}}
  </table>
</body>
</html>`

// WriteHTML renders the role-based report to path.
func WriteHTML(path string, s Summary, results, exceptions []matcher.Result) error {
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, roleData{Summary: s, Results: results, Exceptions: exceptions})
}
