// Package generator produces synthetic Settlement, BankStatement, and
// LedgerEntry records with deliberate, realistic mismatches so the
// matching engine has real noise to work against.
package generator

import (
	"math/rand"
	"time"

	"threefolds/internal/model"
)

// MismatchType classifies how a given transaction's three records relate.
type MismatchType string

const (
	CleanMatch         MismatchType = "clean_match"       // all three agree, straightforward
	UnitOnlyDifference MismatchType = "unit_only"         // paisa/rupee rounding only
	FeeAdjusted        MismatchType = "fee_adjusted"      // net differs from gross by fee+tax
	TimingLagged       MismatchType = "timing_lagged"     // dates differ by a few days
	Batched            MismatchType = "batched"           // multiple settlements, one bank line
	GenuineException   MismatchType = "genuine_exception" // no bank match at all
)

// GroundTruthEntry records the intended outcome for one synthetic
// transaction, used later to verify the matcher's reported match rate
// is real rather than eyeballed.
type GroundTruthEntry struct {
	OrderID     string       `json:"order_id"`
	Type        MismatchType `json:"mismatch_type"`
	ShouldMatch bool         `json:"should_match"`
	Explanation string       `json:"explanation"`
}

// Dataset bundles all generated records plus the ground truth used to
// score the matcher afterward.
type Dataset struct {
	Settlements    []model.Settlement
	BankStatements []model.BankStatement
	LedgerEntries  []model.LedgerEntry
	GroundTruth    []GroundTruthEntry
}

// mismatchDistribution defines roughly what fraction of the batch falls
// into each category. Weighted toward clean/fee-adjusted since that's
// realistic — most transactions DO reconcile cleanly in practice.
var mismatchDistribution = []struct {
	Type   MismatchType
	Weight int
}{
	{CleanMatch, 35},
	{FeeAdjusted, 20},
	{UnitOnlyDifference, 15},
	{TimingLagged, 15},
	{Batched, 8},
	{GenuineException, 7},
}

// Generate produces n synthetic transactions (n should be >= 50 per the
// track's batch requirement) with the mismatch distribution above.
func Generate(n int, seed int64) Dataset {
	rng := rand.New(rand.NewSource(seed))
	ds := Dataset{}

	total := 0
	for _, m := range mismatchDistribution {
		total += m.Weight
	}

	baseTime := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

	for i := 0; i < n; i++ {
		mtype := pickWeighted(rng, total)
		orderID := randomID(rng, "order_", 10)
		paymentID := randomID(rng, "pay_", 14)
		settlementID := randomID(rng, "stl_", 14)
		grossPaisa := int64(50000 + rng.Intn(950000)) // ₹500 - ₹10,000
		feePaisa := grossPaisa * 2 / 100              // 2% fee
		taxPaisa := feePaisa * 18 / 100               // 18% GST on fee
		netPaisa := grossPaisa - feePaisa - taxPaisa
		settledAt := baseTime.Add(time.Duration(i) * time.Hour)

		settlement := model.Settlement{
			SettlementID:     settlementID,
			PaymentID:        paymentID,
			OrderID:          orderID,
			GrossAmountPaisa: grossPaisa,
			FeePaisa:         feePaisa,
			TaxPaisa:         taxPaisa,
			NetAmountPaisa:   netPaisa,
			SettledAt:        settledAt,
			Status:           model.Settlement_Processed,
		}
		ds.Settlements = append(ds.Settlements, settlement)

		ledger := model.LedgerEntry{
			OrderID:                orderID,
			CustomerID:             randomID(rng, "cust_", 8),
			GrossAmountINR:         float64(grossPaisa) / 100,
			ExpectedSettlementDate: settledAt,
			InternalStatus:         "paid",
		}

		var bank *model.BankStatement
		var explanation string

		switch mtype {
		case CleanMatch:
			b := model.BankStatement{
				UTRRef:          randomID(rng, "UTR", 12),
				Narration:       "NEFT/" + paymentID + "/RAZORPAY SETTLEMENT",
				CreditAmountINR: float64(netPaisa) / 100,
				ValueDate:       settledAt,
			}
			bank = &b
			explanation = "exact net amount, narration contains payment id, same-day value date"

		case FeeAdjusted:
			b := model.BankStatement{
				UTRRef:          randomID(rng, "UTR", 12),
				Narration:       "NEFT/RAZORPAY/SETTLEMENT " + orderID,
				CreditAmountINR: float64(netPaisa) / 100,
				ValueDate:       settledAt,
			}
			bank = &b
			explanation = "amount only matches after subtracting fee+tax from gross; must reconcile net not gross"

		case UnitOnlyDifference:
			// Bank rounds to nearest rupee.
			roundedINR := float64(int64(float64(netPaisa)/100 + 0.5))
			b := model.BankStatement{
				UTRRef:          randomID(rng, "UTR", 12),
				Narration:       "NEFT/" + paymentID,
				CreditAmountINR: roundedINR,
				ValueDate:       settledAt,
			}
			bank = &b
			explanation = "bank rounds paisa to nearest rupee; needs tolerance-based amount comparison"

		case TimingLagged:
			b := model.BankStatement{
				UTRRef:          randomID(rng, "UTR", 12),
				Narration:       "NEFT/" + paymentID,
				CreditAmountINR: float64(netPaisa) / 100,
				ValueDate:       settledAt.Add(72 * time.Hour), // 3-day bank lag
			}
			bank = &b
			explanation = "value date lags settled_at by 3 days; within acceptable timing window"

		case Batched:
			b := model.BankStatement{
				UTRRef:          randomID(rng, "UTR", 12),
				Narration:       "NEFT/RAZORPAY BATCH SETTLEMENT",
				CreditAmountINR: float64(netPaisa)/100 + 1250.00, // bundled with another settlement
				ValueDate:       settledAt,
				BatchFlag:       true,
			}
			bank = &b
			explanation = "bank credit bundles this settlement with another; amount won't match 1:1, needs batch-aware matching"

		case GenuineException:
			bank = nil // no corresponding bank record at all
			ledger.InternalStatus = "pending"
			explanation = "no bank record found within any reasonable window; likely reversed or delayed beyond batch cutoff"
		}

		if bank != nil {
			ds.BankStatements = append(ds.BankStatements, *bank)
		}
		ds.LedgerEntries = append(ds.LedgerEntries, ledger)

		ds.GroundTruth = append(ds.GroundTruth, GroundTruthEntry{
			OrderID:     orderID,
			Type:        mtype,
			ShouldMatch: mtype != GenuineException,
			Explanation: explanation,
		})
	}

	return ds
}

func pickWeighted(rng *rand.Rand, total int) MismatchType {
	r := rng.Intn(total)
	cum := 0
	for _, m := range mismatchDistribution {
		cum += m.Weight
		if r < cum {
			return m.Type
		}
	}
	return CleanMatch
}

func randomID(rng *rand.Rand, prefix string, n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rng.Intn(len(chars))]
	}
	return prefix + string(b)
}