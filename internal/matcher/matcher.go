package matcher

import (
	"fmt"
	"math"
	"strings"
	"time"

	"threefolds/internal/model"
)

type Tier string

const (
	TierExact      Tier = "exact"
	TierFuzzy      Tier = "fuzzy"
	TierBatch      Tier = "batch"
	TierUnresolved Tier = "unresolved"
)

const (
	amountToleranceINR = 2.0
	dateToleranceHours = 5 * 24 * time.Hour
)

type Result struct {
	OrderID      string `json:"order_id"`
	SettlementID string `json:"settlement_id"`
	BankUTRRef   string `json:"bank_utr_ref,omitempty"`
	Tier         Tier   `json:"tier"`

	BatchSettlementIDs []string `json:"batch_settlement_ids,omitempty"`

	AmountDiffINR float64 `json:"amount_diff_inr"`
	DateDiffHours float64 `json:"date_diff_hours"`

	LedgerFound       bool    `json:"ledger_found"`
	LedgerAmountDiff  float64 `json:"ledger_amount_diff_inr,omitempty"`
	LedgerDateDiffHrs float64 `json:"ledger_date_diff_hours,omitempty"`
	LedgerStatus      string  `json:"ledger_status,omitempty"`

	Reason string `json:"reason"`
}

func Match(
	settlements []model.Settlement,
	bankStatements []model.BankStatement,
	ledgerEntries []model.LedgerEntry,
) []Result {

	ledgerByOrder := make(map[string]model.LedgerEntry)

	for _, ledger := range ledgerEntries {
		ledgerByOrder[ledger.OrderID] = ledger
	}

	usedBanks := make(map[string]bool)
	usedSettlements := make(map[string]bool)

	results := make([]Result, 0, len(settlements))

	// ------------------------------------------------------------
	// PASS 1: exact payment-ID matches
	// ------------------------------------------------------------

	for _, settlement := range settlements {
		if usedSettlements[settlement.SettlementID] {
			continue
		}

		bank, ok := findExactMatch(
			settlement,
			bankStatements,
			usedBanks,
		)

		if !ok {
			continue
		}

		result := buildResult(
			settlement,
			bank,
			ledgerByOrder,
			TierExact,
			"payment ID, amount and bank evidence match",
		)

		results = append(results, result)

		usedBanks[bank.UTRRef] = true
		usedSettlements[settlement.SettlementID] = true
	}

	// ------------------------------------------------------------
	// PASS 2: real batch matches
	// ------------------------------------------------------------

	for i := 0; i < len(settlements); i++ {
		a := settlements[i]

		if usedSettlements[a.SettlementID] {
			continue
		}

		for j := i + 1; j < len(settlements); j++ {
			b := settlements[j]

			if usedSettlements[b.SettlementID] {
				continue
			}

			bank, ok := findBatchMatch(
				a,
				b,
				bankStatements,
				usedBanks,
			)

			if !ok {
				continue
			}

			results = append(results,
				buildBatchResult(
					a,
					b,
					bank,
					ledgerByOrder,
				),
			)
			results = append(results,

				buildBatchResult(
					b,
					a,
					bank,
					ledgerByOrder,
				),
			)

			usedBanks[bank.UTRRef] = true
			usedSettlements[a.SettlementID] = true
			usedSettlements[b.SettlementID] = true

			break
		}
	}

	// ------------------------------------------------------------
	// PASS 3: fuzzy one-to-one matches
	// ------------------------------------------------------------

	for _, settlement := range settlements {
		if usedSettlements[settlement.SettlementID] {
			continue
		}

		bank, amountDiff, dateDiff, ok := findBestFuzzyMatch(
			settlement,
			bankStatements,
			usedBanks,
		)

		if !ok {
			continue
		}

		result := buildResult(
			settlement,
			bank,
			ledgerByOrder,
			TierFuzzy,
			fmt.Sprintf(
				"fuzzy match: amount difference ₹%.2f, date difference %.1f hours",
				amountDiff,
				dateDiff,
			),
		)

		result.AmountDiffINR = amountDiff
		result.DateDiffHours = dateDiff

		results = append(results, result)

		usedBanks[bank.UTRRef] = true
		usedSettlements[settlement.SettlementID] = true
	}

	// ------------------------------------------------------------
	// PASS 4: unresolved
	// ------------------------------------------------------------

	for _, settlement := range settlements {
		if usedSettlements[settlement.SettlementID] {
			continue
		}

		result := buildUnresolvedResult(
			settlement,
			ledgerByOrder,
		)

		results = append(results, result)
	}

	return results
}

// ------------------------------------------------------------
// Exact matching
// ------------------------------------------------------------

func findExactMatch(
	settlement model.Settlement,
	bankStatements []model.BankStatement,
	usedBanks map[string]bool,
) (model.BankStatement, bool) {

	settlementAmount := float64(settlement.NetAmountPaisa) / 100

	for _, bank := range bankStatements {
		if usedBanks[bank.UTRRef] {
			continue
		}

		if bank.BatchFlag {
			continue
		}

		// Payment ID must be present.
		if !strings.Contains(bank.Narration, settlement.PaymentID) {
			continue
		}

		// Bank amount must match the settlement net amount.
		amountDiff := math.Abs(
			settlementAmount - bank.CreditAmountINR,
		)

		if amountDiff > amountToleranceINR {
			continue
		}

		// Bank date must be reasonably close to settlement date.
		dateDiff := math.Abs(
			bank.ValueDate.Sub(settlement.SettledAt).Hours(),
		)

		if dateDiff > 24 {
			continue
		}

		return bank, true
	}

	return model.BankStatement{}, false
}

// ------------------------------------------------------------
// Batch matching
// ------------------------------------------------------------

func findBatchMatch(
	a model.Settlement,
	b model.Settlement,
	bankStatements []model.BankStatement,
	usedBanks map[string]bool,
) (model.BankStatement, bool) {

	targetPaisa := a.NetAmountPaisa + b.NetAmountPaisa

	for _, bank := range bankStatements {
		if usedBanks[bank.UTRRef] {
			continue
		}

		if !bank.BatchFlag {
			continue
		}

		bankPaisa := int64(math.Round(
			bank.CreditAmountINR * 100,
		))

		if bankPaisa != targetPaisa {
			continue
		}

		// Make sure the bank date is reasonably related
		// to at least one settlement in the batch.
		dateA := bank.ValueDate.Sub(a.SettledAt)
		dateB := bank.ValueDate.Sub(b.SettledAt)

		if math.Abs(dateA.Hours()) > 5*24 &&
			math.Abs(dateB.Hours()) > 5*24 {
			continue
		}

		return bank, true
	}

	return model.BankStatement{}, false
}

// ------------------------------------------------------------
// Batch result
// ------------------------------------------------------------

func buildBatchResult(
	settlement model.Settlement,
	other model.Settlement,
	bank model.BankStatement,
	ledgerByOrder map[string]model.LedgerEntry,
) Result {

	result := Result{
		OrderID:      settlement.OrderID,
		SettlementID: settlement.SettlementID,
		BankUTRRef:   bank.UTRRef,
		Tier:         TierBatch,
		BatchSettlementIDs: []string{
			settlement.SettlementID,
			other.SettlementID,
		},
		AmountDiffINR: 0,
		DateDiffHours: math.Abs(
			bank.ValueDate.Sub(settlement.SettledAt).Hours(),
		),
		Reason: fmt.Sprintf(
			"batch match: settlement %s + settlement %s reconcile to bank batch %s",
			settlement.SettlementID,
			other.SettlementID,
			bank.UTRRef,
		),
	}

	if ledger, ok := ledgerByOrder[settlement.OrderID]; ok {
		result.LedgerFound = true

		settlementGrossINR :=
			float64(settlement.GrossAmountPaisa) / 100

		result.LedgerAmountDiff =
			math.Abs(settlementGrossINR - ledger.GrossAmountINR)

		result.LedgerDateDiffHrs =
			math.Abs(
				ledger.ExpectedSettlementDate.
					Sub(settlement.SettledAt).
					Hours(),
			)

		result.LedgerStatus = ledger.InternalStatus
	}

	return result
}

// ------------------------------------------------------------
// Fuzzy matching
// ------------------------------------------------------------

func findBestFuzzyMatch(
	settlement model.Settlement,
	bankStatements []model.BankStatement,
	usedBanks map[string]bool,
) (model.BankStatement, float64, float64, bool) {

	var best model.BankStatement

	bestScore := math.MaxFloat64
	bestAmountDiff := 0.0
	bestDateDiff := 0.0
	found := false

	settlementAmount := float64(settlement.NetAmountPaisa) / 100

	for _, bank := range bankStatements {
		if usedBanks[bank.UTRRef] {
			continue
		}

		if bank.BatchFlag {
			continue
		}

		amountDiff := math.Abs(
			settlementAmount - bank.CreditAmountINR,
		)

		dateDiff := math.Abs(
			bank.ValueDate.Sub(settlement.SettledAt).Hours(),
		)

		if amountDiff > amountToleranceINR {
			continue
		}

		if dateDiff > dateToleranceHours.Hours() {
			continue
		}

		score := amountDiff*10 + dateDiff

		if score < bestScore {
			bestScore = score
			best = bank
			bestAmountDiff = amountDiff
			bestDateDiff = dateDiff
			found = true
		}
	}

	return best, bestAmountDiff, bestDateDiff, found
}

// ------------------------------------------------------------
// Result builders
// ------------------------------------------------------------

func buildResult(
	settlement model.Settlement,
	bank model.BankStatement,
	ledgerByOrder map[string]model.LedgerEntry,
	tier Tier,
	reason string,
) Result {

	result := Result{
		OrderID:      settlement.OrderID,
		SettlementID: settlement.SettlementID,
		BankUTRRef:   bank.UTRRef,
		Tier:         tier,
		Reason:       reason,
	}

	if ledger, ok := ledgerByOrder[settlement.OrderID]; ok {
		result.LedgerFound = true

		settlementGrossINR :=
			float64(settlement.GrossAmountPaisa) / 100

		result.LedgerAmountDiff =
			math.Abs(settlementGrossINR - ledger.GrossAmountINR)

		result.LedgerDateDiffHrs =
			math.Abs(
				ledger.ExpectedSettlementDate.
					Sub(settlement.SettledAt).
					Hours(),
			)

		result.LedgerStatus = ledger.InternalStatus
	}

	return result
}

func buildUnresolvedResult(
	settlement model.Settlement,
	ledgerByOrder map[string]model.LedgerEntry,
) Result {

	result := Result{
		OrderID:      settlement.OrderID,
		SettlementID: settlement.SettlementID,
		Tier:         TierUnresolved,
		Reason:       "no valid bank match found",
	}

	if ledger, ok := ledgerByOrder[settlement.OrderID]; ok {
		result.LedgerFound = true

		settlementGrossINR :=
			float64(settlement.GrossAmountPaisa) / 100

		result.LedgerAmountDiff =
			math.Abs(settlementGrossINR - ledger.GrossAmountINR)

		result.LedgerDateDiffHrs =
			math.Abs(
				ledger.ExpectedSettlementDate.
					Sub(settlement.SettledAt).
					Hours(),
			)

		result.LedgerStatus = ledger.InternalStatus
	}

	return result
}

// ------------------------------------------------------------
// Metrics
// ------------------------------------------------------------

func MatchRate(results []Result) float64 {
	if len(results) == 0 {
		return 0
	}

	matched := 0

	for _, r := range results {
		if r.Tier != TierUnresolved {
			matched++
		}
	}

	return float64(matched) / float64(len(results))
}
