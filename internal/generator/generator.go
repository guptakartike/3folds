package generator

import (
	"fmt"
	"math/rand"
	"time"

	"threefolds/internal/model"
)

type Scenario string

const (
	Clean       Scenario = "clean"
	FeeAdjusted Scenario = "fee_adjusted"
	UnitDiff    Scenario = "unit_difference"
	TimingLag   Scenario = "timing_lag"
	Batched     Scenario = "batched"
	Exception   Scenario = "genuine_exception"
)

type GroundTruthEntry struct {
	OrderID     string   `json:"order_id"`
	Type        Scenario `json:"type"`
	ShouldMatch bool     `json:"should_match"`
	Explanation string   `json:"explanation"`
}

type Dataset struct {
	Settlements    []model.Settlement
	BankStatements []model.BankStatement
	LedgerEntries  []model.LedgerEntry
	GroundTruth    []GroundTruthEntry
}

func Generate(n int, seed int64) Dataset {
	rng := rand.New(rand.NewSource(seed))

	ds := Dataset{
		Settlements:    make([]model.Settlement, 0, n),
		BankStatements: make([]model.BankStatement, 0, n),
		LedgerEntries:  make([]model.LedgerEntry, 0, n),
		GroundTruth:    make([]GroundTruthEntry, 0, n),
	}

	// We generate the settlements and ledger first.
	// This allows the batching scenario to combine two REAL settlements.
	for i := 0; i < n; i++ {
		settlement, ledger := createSettlementAndLedger(rng, i)

		ds.Settlements = append(ds.Settlements, settlement)
		ds.LedgerEntries = append(ds.LedgerEntries, ledger)
	}

	// Keep track of settlements already consumed by a batch.
	batched := make(map[string]bool)

	for i, settlement := range ds.Settlements {
		if batched[settlement.SettlementID] {
			continue
		}

		scenario := chooseScenario(i, n)

		// A batch requires another real settlement.
		if scenario == Batched && i+1 < n && !batched[ds.Settlements[i+1].SettlementID] {
			other := ds.Settlements[i+1]

			bank := createBatchBankRecord(
				rng,
				settlement,
				other,
			)

			ds.BankStatements = append(ds.BankStatements, bank)

			ds.GroundTruth = append(ds.GroundTruth,
				GroundTruthEntry{
					OrderID:     settlement.OrderID,
					Type:        Batched,
					ShouldMatch: true,
					Explanation: "two real settlements are combined into one bank credit and require multi-settlement reconciliation",
				},
				GroundTruthEntry{
					OrderID:     other.OrderID,
					Type:        Batched,
					ShouldMatch: true,
					Explanation: "two real settlements are combined into one bank credit and require multi-settlement reconciliation",
				},
			)

			batched[settlement.SettlementID] = true
			batched[other.SettlementID] = true
			continue
		}

		bank, shouldMatch, explanation := createBankRecord(
			rng,
			settlement,
			scenario,
		)

		if bank != nil {
			ds.BankStatements = append(ds.BankStatements, *bank)
		}

		ds.GroundTruth = append(ds.GroundTruth, GroundTruthEntry{
			OrderID:     settlement.OrderID,
			Type:        scenario,
			ShouldMatch: shouldMatch,
			Explanation: explanation,
		})
	}

	return ds
}

func createSettlementAndLedger(
	rng *rand.Rand,
	index int,
) (model.Settlement, model.LedgerEntry) {

	orderID := fmt.Sprintf("order_%06d", index+1)
	paymentID := randomID(rng, "pay_", 12)

	grossPaisa := int64(100000 + rng.Intn(900000)) // ₹1,000–₹10,000

	feePaisa := grossPaisa * 2 / 100
	taxPaisa := feePaisa * 18 / 100
	netPaisa := grossPaisa - feePaisa - taxPaisa

	settledAt := randomDate(rng)

	settlement := model.Settlement{
		SettlementID:     randomID(rng, "set_", 12),
		PaymentID:        paymentID,
		OrderID:          orderID,
		GrossAmountPaisa: grossPaisa,
		FeePaisa:         feePaisa,
		TaxPaisa:         taxPaisa,
		NetAmountPaisa:   netPaisa,
		SettledAt:        settledAt,
		Status:           model.Settlement_Status("settled"),
	}

	ledger := model.LedgerEntry{
		OrderID:                orderID,
		CustomerID:             randomID(rng, "cust_", 10),
		GrossAmountINR:         float64(grossPaisa) / 100,
		ExpectedSettlementDate: settledAt,
		InternalStatus:         "paid",
	}

	return settlement, ledger
}

func chooseScenario(index, n int) Scenario {
	// Deterministic distribution approximately matching the PS:
	//
	// ~35% clean
	// ~20% fee adjusted
	// ~15% unit difference
	// ~15% timing lag
	// ~8% batched
	// ~7% genuine exception

	ratio := float64(index) / float64(n)

	switch {
	case ratio < 0.35:
		return Clean
	case ratio < 0.55:
		return FeeAdjusted
	case ratio < 0.70:
		return UnitDiff
	case ratio < 0.85:
		return TimingLag
	case ratio < 0.93:
		return Batched
	default:
		return Exception
	}
}

func createBankRecord(
	rng *rand.Rand,
	settlement model.Settlement,
	scenario Scenario,
) (*model.BankStatement, bool, string) {

	netPaisa := settlement.NetAmountPaisa
	settledAt := settlement.SettledAt

	bank := model.BankStatement{
		UTRRef:          randomID(rng, "UTR", 12),
		Narration:       "NEFT/RAZORPAY/" + settlement.PaymentID,
		CreditAmountINR: float64(netPaisa) / 100,
		ValueDate:       settledAt,
		BatchFlag:       false,
	}

	switch scenario {

	case Clean:
		return &bank,
			true,
			"exact payment ID, amount and settlement date match"

	case FeeAdjusted:
		// Deliberately introduce a small amount difference.
		// ₹1 difference remains inside fuzzy tolerance.
		adjustment := int64(100)

		bank.CreditAmountINR =
			float64(netPaisa-adjustment) / 100

		return &bank,
			true,
			"payment ID and date match, but bank credit differs by ₹1"

	case UnitDiff:
		// Introduce a small rounding/unit representation difference.
		// This is intentionally different from the settlement amount
		// but still inside fuzzy tolerance.
		adjustment := int64(50) // ₹0.50

		bank.CreditAmountINR =
			float64(netPaisa-adjustment) / 100

		return &bank,
			true,
			"bank amount differs by ₹0.50 due to unit/rounding representation"

	case TimingLag:
		// Keep the amount exact but move the bank date by 6 days.
		// This exceeds the exact date tolerance and also exceeds
		// the current fuzzy date tolerance, so it should remain
		// unresolved unless the fuzzy rules are relaxed.
		bank.ValueDate = settledAt.Add(3 * 24 * time.Hour)

		return &bank,
			true,
			"payment ID and amount match, but bank value date is delayed by 6 days"

	case Exception:
		// Genuine exception: unrelated bank transaction.
		bank.Narration =
			"NEFT/UNKNOWN/" + randomID(rng, "pay_", 12)

		bank.CreditAmountINR =
			float64(netPaisa+int64(50000+rng.Intn(100000))) / 100

		return &bank,
			false,
			"no corresponding bank transaction exists for this settlement"

	default:
		return nil,
			false,
			"unsupported scenario"
	}
}
func createBatchBankRecord(
	rng *rand.Rand,
	settlementA model.Settlement,
	settlementB model.Settlement,
) model.BankStatement {

	totalNetPaisa :=
		settlementA.NetAmountPaisa +
			settlementB.NetAmountPaisa

	batchID := randomID(rng, "batch_", 10)

	// Use the earlier settlement date as the bank value date.
	valueDate := settlementA.SettledAt
	if settlementB.SettledAt.Before(valueDate) {
		valueDate = settlementB.SettledAt
	}

	return model.BankStatement{
		UTRRef:          randomID(rng, "UTR", 12),
		Narration:       "NEFT/RAZORPAY/BATCH/" + batchID,
		CreditAmountINR: float64(totalNetPaisa) / 100,
		ValueDate:       valueDate,
		BatchFlag:       true,
		BatchID:         batchID,
	}
}

func randomID(rng *rand.Rand, prefix string, length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

	b := make([]byte, length)

	for i := range b {
		b[i] = chars[rng.Intn(len(chars))]
	}

	return prefix + string(b)
}

func randomDate(rng *rand.Rand) time.Time {
	base := time.Date(
		2026,
		1,
		1,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	days := rng.Intn(60)

	return base.AddDate(0, 0, days)
}