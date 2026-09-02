// Package matcher implements the exact and fuzzy matching tiers that
// reconcile Settlement records against BankStatement records. Anything
// neither tier can resolve is left for the (separate) LLM exception
// resolver.
package matcher

import (
	"fmt"
	"math"
	"strings"
	"time"

	"threefolds/internal/model"
)

// Tier identifies which stage resolved (or failed to resolve) a match.
type Tier string

const (
	TierExact      Tier = "exact"      // payment id found verbatim in narration
	TierFuzzy      Tier = "fuzzy"      // resolved by amount + date scoring
	TierUnresolved Tier = "unresolved" // needs the LLM exception resolver
)

// Tolerances for fuzzy matching. Deliberately generous enough to absorb
// rounding and normal bank lag, but tight enough that batched credits
// (which differ by more than this) correctly fall through to unresolved.
const (
	amountToleranceINR = 2.0                // absorbs paisa-to-rupee rounding
	dateToleranceHours = 5 * 24 * time.Hour // absorbs normal settlement lag
)

// Result is the outcome of trying to match one settlement.
type Result struct {
	OrderID       string  `json:"order_id"`
	SettlementID  string  `json:"settlement_id"`
	BankUTRRef    string  `json:"bank_utr_ref,omitempty"` // empty if unresolved
	Tier          Tier    `json:"tier"`
	AmountDiffINR float64 `json:"amount_diff_inr"`
	DateDiffHours float64 `json:"date_diff_hours"`
	Reason        string  `json:"reason"`
}

// Match reconciles settlements against bank statements. It does not
// mutate its inputs.
func Match(settlements []model.Settlement, bankStatements []model.BankStatement) []Result {
	used := make([]bool, len(bankStatements))
	results := make([]Result, 0, len(settlements))

	for _, s := range settlements {
		netINR := float64(s.NetAmountPaisa) / 100

		// Tier 1: exact — payment id appears verbatim in narration, and
		// that bank record hasn't already been claimed.
		if idx := findExactMatch(s, bankStatements, used); idx >= 0 {
			b := bankStatements[idx]
			used[idx] = true
			results = append(results, Result{
				OrderID:       s.OrderID,
				SettlementID:  s.SettlementID,
				BankUTRRef:    b.UTRRef,
				Tier:          TierExact,
				AmountDiffINR: math.Abs(b.CreditAmountINR - netINR),
				DateDiffHours: b.ValueDate.Sub(s.SettledAt).Hours(),
				Reason:        "payment id found in bank narration",
			})
			continue
		}

		// Tier 2: fuzzy — best-scoring unused candidate within tolerance
		// on both amount and date.
		if idx, amtDiff, dateDiff := findBestFuzzyMatch(s, bankStatements, used, netINR); idx >= 0 {
			b := bankStatements[idx]
			used[idx] = true
			results = append(results, Result{
				OrderID:       s.OrderID,
				SettlementID:  s.SettlementID,
				BankUTRRef:    b.UTRRef,
				Tier:          TierFuzzy,
				AmountDiffINR: amtDiff,
				DateDiffHours: dateDiff.Hours(),
				Reason: fmt.Sprintf(
					"amount within ₹%.2f and date within %.0fh of settlement, no exact id match",
					amtDiff, dateDiff.Hours(),
				),
			})
			continue
		}

		// Tier 3: unresolved — no candidate satisfies either tier. This
		// is either a genuine exception, or a case (like batching) that
		// needs the LLM resolver's judgment.
		results = append(results, Result{
			OrderID:      s.OrderID,
			SettlementID: s.SettlementID,
			Tier:         TierUnresolved,
			Reason:       "no bank record found within amount/date tolerance",
		})
	}

	return results
}

func findExactMatch(s model.Settlement, bankStatements []model.BankStatement, used []bool) int {
	for i, b := range bankStatements {
		if used[i] {
			continue
		}
		if strings.Contains(b.Narration, s.PaymentID) {
			return i
		}
	}
	return -1
}

// findBestFuzzyMatch scores every unused bank statement and returns the
// index of the best candidate within tolerance, or -1 if none qualifies.
func findBestFuzzyMatch(s model.Settlement, bankStatements []model.BankStatement, used []bool, netINR float64) (int, float64, time.Duration) {
	bestIdx := -1
	bestScore := math.MaxFloat64
	var bestAmtDiff float64
	var bestDateDiff time.Duration

	for i, b := range bankStatements {
		if used[i] {
			continue
		}

		amtDiff := math.Abs(b.CreditAmountINR - netINR)
		dateDiff := b.ValueDate.Sub(s.SettledAt)
		if dateDiff < 0 {
			dateDiff = -dateDiff
		}

		if amtDiff > amountToleranceINR || dateDiff > dateToleranceHours {
			continue // outside tolerance, not a candidate at all
		}

		// Lower is better: weight amount more heavily than date since
		// amount agreement is the stronger signal.
		score := amtDiff*10 + dateDiff.Hours()
		if score < bestScore {
			bestScore = score
			bestIdx = i
			bestAmtDiff = amtDiff
			bestDateDiff = dateDiff
		}
	}

	return bestIdx, bestAmtDiff, bestDateDiff
}

// MatchRate returns the fraction of results resolved by either tier
// (exact or fuzzy), as required by the track's "measured accuracy" bar.
func MatchRate(results []Result) float64 {
	if len(results) == 0 {
		return 0
	}
	resolved := 0
	for _, r := range results {
		if r.Tier == TierExact || r.Tier == TierFuzzy {
			resolved++
		}
	}
	return float64(resolved) / float64(len(results))
}