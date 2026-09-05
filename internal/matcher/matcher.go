package matcher

import (
	"fmt"
	"math"
	"strings"

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
	fuzzyDateDays      = 5
	exactDateHours     = 24
)

type Result struct {
	OrderID            string   `json:"order_id"`
	SettlementID       string   `json:"settlement_id"`
	BankUTRRef         string   `json:"bank_utr_ref"`
	Tier               Tier     `json:"tier"`
	BatchSettlementIDs []string `json:"batch_settlement_ids,omitempty"`
	AmountDiffINR      float64  `json:"amount_diff_inr"`
	DateDiffHours      float64  `json:"date_diff_hours"`
	LedgerFound        bool     `json:"ledger_found"`
	LedgerAmountDiff   float64  `json:"ledger_amount_diff"`
	LedgerDateDiffHrs  float64  `json:"ledger_date_diff_hours"`
	LedgerStatus       string   `json:"ledger_status"`
	Reason             string   `json:"reason"`
}

// Match reconciles settlements against bank statements and validates
// candidates against the merchant ledger.
//
// Matching strategy:
//  1. Exact payment-ID + amount + date
//  2. Batch amount matching
//  3. Fuzzy amount + date matching
//  4. Unresolved
func Match(
	settlements []model.Settlement,
	bankStatements []model.BankStatement,
	ledgerEntries []model.LedgerEntry,
) []Result {

	ledgerByOrder := make(map[string]model.LedgerEntry, len(ledgerEntries))

	for _, ledger := range ledgerEntries {
		ledgerByOrder[ledger.OrderID] = ledger
	}

	usedBanks := make(map[string]bool)
	usedSettlements := make(map[string]bool)

	// Build indexes once instead of repeatedly scanning all bank statements.
	paymentIndex := buildPaymentIDIndex(bankStatements)
	batchIndex := buildBatchIndex(bankStatements)

	results := make([]Result, 0, len(settlements))

	// ------------------------------------------------------------
	// PASS 1: EXACT MATCH
	// ------------------------------------------------------------

	for _, settlement := range settlements {
		if usedSettlements[settlement.SettlementID] {
			continue
		}

		bank, ok := findExactMatch(
			settlement,
			paymentIndex,
			usedBanks,
			ledgerByOrder,
		)

		if !ok {
			continue
		}

		result := buildExactResult(
			settlement,
			bank,
			ledgerByOrder,
		)

		results = append(results, result)

		usedSettlements[settlement.SettlementID] = true
		usedBanks[bank.UTRRef] = true
	}

	// ------------------------------------------------------------
	// PASS 2: BATCH MATCH
	// ------------------------------------------------------------

	for i := 0; i < len(settlements); i++ {
		a := settlements[i]

		if usedSettlements[a.SettlementID] {
			continue
		}

		if !ledgerSupportsSettlement(a, ledgerByOrder) {
			continue
		}

		for j := i + 1; j < len(settlements); j++ {
			b := settlements[j]

			if usedSettlements[b.SettlementID] {
				continue
			}

			if !ledgerSupportsSettlement(b, ledgerByOrder) {
				continue
			}

			bank, ok := findBatchMatch(
				a,
				b,
				batchIndex,
				usedBanks,
			)

			if !ok {
				continue
			}

			resultA := buildBatchResult(
				a,
				b,
				bank,
				ledgerByOrder,
			)

			resultB := buildBatchResult(
				b,
				a,
				bank,
				ledgerByOrder,
			)

			results = append(results, resultA, resultB)

			usedBanks[bank.UTRRef] = true
			usedSettlements[a.SettlementID] = true
			usedSettlements[b.SettlementID] = true

			break
		}
	}

	// ------------------------------------------------------------
	// PASS 3: FUZZY MATCH
	// ------------------------------------------------------------

	fuzzyIndex := buildAmountIndex(bankStatements)

	for _, settlement := range settlements {
		if usedSettlements[settlement.SettlementID] {
			continue
		}

		if !ledgerSupportsSettlement(settlement, ledgerByOrder) {
			continue
		}

		bank, ok := findBestFuzzyMatch(
			settlement,
			fuzzyIndex,
			usedBanks,
		)

		if !ok {
			continue
		}

		result := buildFuzzyResult(
			settlement,
			bank,
			ledgerByOrder,
		)

		results = append(results, result)

		usedSettlements[settlement.SettlementID] = true
		usedBanks[bank.UTRRef] = true
	}

	// ------------------------------------------------------------
	// PASS 4: UNRESOLVED
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

		usedSettlements[settlement.SettlementID] = true
	}

	return results
}

// ------------------------------------------------------------
// INDEXES
// ------------------------------------------------------------

// buildPaymentIDIndex indexes non-batch bank statements by payment ID.
//
// Bank narration is expected to contain the payment ID, e.g.
// "Razorpay settlement payment_000001".
func buildPaymentIDIndex(
	bankStatements []model.BankStatement,
) map[string][]model.BankStatement {

	index := make(map[string][]model.BankStatement)

	for _, bank := range bankStatements {
		if bank.BatchFlag {
			continue
		}

		paymentID := extractPaymentID(bank.Narration)

		if paymentID == "" {
			continue
		}

		index[paymentID] = append(index[paymentID], bank)
	}

	return index
}

// buildBatchIndex indexes batch bank statements by their amount in paisa.
//
// This eliminates the expensive scan through every bank statement
// for every settlement pair.
func buildBatchIndex(
	bankStatements []model.BankStatement,
) map[int64][]model.BankStatement {

	index := make(map[int64][]model.BankStatement)

	for _, bank := range bankStatements {
		if !bank.BatchFlag {
			continue
		}

		amountPaisa := int64(
			math.Round(bank.CreditAmountINR * 100),
		)

		index[amountPaisa] = append(
			index[amountPaisa],
			bank,
		)
	}

	return index
}

// buildAmountIndex creates an amount-bucket index used by fuzzy matching.
//
// Amounts are stored in paisa. Fuzzy matching only needs to inspect
// buckets within ±₹2 instead of scanning every bank statement.
func buildAmountIndex(
	bankStatements []model.BankStatement,
) map[int64][]model.BankStatement {

	index := make(map[int64][]model.BankStatement)

	for _, bank := range bankStatements {
		if bank.BatchFlag {
			continue
		}

		amountPaisa := int64(
			math.Round(bank.CreditAmountINR * 100),
		)

		index[amountPaisa] = append(
			index[amountPaisa],
			bank,
		)
	}

	return index
}

// extractPaymentID extracts the payment ID from bank narration.
//
// The generated data uses narrations containing the payment ID.
// We locate the token beginning with "payment_".
func extractPaymentID(narration string) string {
	const prefix = "pay_"

	start := strings.Index(narration, prefix)
	if start == -1 {
		return ""
	}

	end := start

	for end < len(narration) {
		c := narration[end]

		if c == '/' || c == ' ' || c == ',' {
			break
		}

		end++
	}

	return narration[start:end]
}

// ------------------------------------------------------------
// LEDGER VALIDATION
// ------------------------------------------------------------

func ledgerSupportsSettlement(
	settlement model.Settlement,
	ledgerByOrder map[string]model.LedgerEntry,
) bool {

	ledger, ok := ledgerByOrder[settlement.OrderID]

	if !ok {
		return false
	}

	if math.Abs(
		ledger.GrossAmountINR-
			float64(settlement.GrossAmountPaisa)/100,
	) > amountToleranceINR {
		return false
	}

	if strings.ToLower(ledger.InternalStatus) != "paid" {
		return false
	}

	return true
}

func addLedgerEvidence(
	result *Result,
	settlement model.Settlement,
	ledgerByOrder map[string]model.LedgerEntry,
) {

	ledger, ok := ledgerByOrder[settlement.OrderID]

	if !ok {
		result.LedgerFound = false
		return
	}

	result.LedgerFound = true

	settlementGrossINR :=
		float64(settlement.GrossAmountPaisa) / 100

	result.LedgerAmountDiff =
		math.Abs(ledger.GrossAmountINR - settlementGrossINR)

	result.LedgerDateDiffHrs =
		math.Abs(
			ledger.ExpectedSettlementDate.
				Sub(settlement.SettledAt).
				Hours(),
		)

	result.LedgerStatus = ledger.InternalStatus
}

// ------------------------------------------------------------
// EXACT MATCH
// ------------------------------------------------------------

func findExactMatch(
	settlement model.Settlement,
	paymentIndex map[string][]model.BankStatement,
	usedBanks map[string]bool,
	ledgerByOrder map[string]model.LedgerEntry,
) (model.BankStatement, bool) {

	if !ledgerSupportsSettlement(settlement, ledgerByOrder) {
		return model.BankStatement{}, false
	}

	candidates := paymentIndex[settlement.PaymentID]

	expectedAmount :=
		float64(settlement.NetAmountPaisa) / 100

	for _, bank := range candidates {
		if usedBanks[bank.UTRRef] {
			continue
		}

		amountDiff :=
			math.Abs(bank.CreditAmountINR - expectedAmount)

		if amountDiff > amountToleranceINR {
			continue
		}

		dateDiff :=
			math.Abs(
				bank.ValueDate.
					Sub(settlement.SettledAt).
					Hours(),
			)

		if dateDiff > exactDateHours {
			continue
		}

		return bank, true
	}

	return model.BankStatement{}, false
}

// ------------------------------------------------------------
// BATCH MATCH
// ------------------------------------------------------------

func findBatchMatch(
	a model.Settlement,
	b model.Settlement,
	batchIndex map[int64][]model.BankStatement,
	usedBanks map[string]bool,
) (model.BankStatement, bool) {

	targetPaisa :=
		a.NetAmountPaisa +
			b.NetAmountPaisa

	candidates := batchIndex[targetPaisa]

	for _, bank := range candidates {
		if usedBanks[bank.UTRRef] {
			continue
		}

		dateA :=
			math.Abs(
				bank.ValueDate.
					Sub(a.SettledAt).
					Hours(),
			)

		dateB :=
			math.Abs(
				bank.ValueDate.
					Sub(b.SettledAt).
					Hours(),
			)

		if dateA > fuzzyDateDays*24 &&
			dateB > fuzzyDateDays*24 {
			continue
		}

		return bank, true
	}

	return model.BankStatement{}, false
}

// ------------------------------------------------------------
// FUZZY MATCH
// ------------------------------------------------------------

func findBestFuzzyMatch(
	settlement model.Settlement,
	amountIndex map[int64][]model.BankStatement,
	usedBanks map[string]bool,
) (model.BankStatement, bool) {

	expectedAmount :=
		float64(settlement.NetAmountPaisa) / 100

	expectedPaisa := settlement.NetAmountPaisa

	tolerancePaisa :=
		int64(math.Round(amountToleranceINR * 100))

	var best model.BankStatement
	bestScore := math.MaxFloat64
	found := false

	// Search only amount buckets within ±₹2.
	for amountPaisa := expectedPaisa - tolerancePaisa; amountPaisa <= expectedPaisa+tolerancePaisa; amountPaisa++ {

		candidates := amountIndex[amountPaisa]

		for _, bank := range candidates {
			if usedBanks[bank.UTRRef] {
				continue
			}

			amountDiff :=
				math.Abs(
					bank.CreditAmountINR -
						expectedAmount,
				)

			if amountDiff > amountToleranceINR {
				continue
			}

			dateDiff :=
				math.Abs(
					bank.ValueDate.
						Sub(settlement.SettledAt).
						Hours(),
				)

			if dateDiff > fuzzyDateDays*24 {
				continue
			}

			// Preserve the original scoring model.
			score :=
				amountDiff*10 +
					dateDiff

			if score < bestScore {
				bestScore = score
				best = bank
				found = true
			}
		}
	}

	return best, found
}

// ------------------------------------------------------------
// RESULT BUILDERS
// ------------------------------------------------------------

func buildExactResult(
	settlement model.Settlement,
	bank model.BankStatement,
	ledgerByOrder map[string]model.LedgerEntry,
) Result {

	expectedAmount :=
		float64(settlement.NetAmountPaisa) / 100

	result := Result{
		OrderID:       settlement.OrderID,
		SettlementID:  settlement.SettlementID,
		BankUTRRef:    bank.UTRRef,
		Tier:          TierExact,
		AmountDiffINR: math.Abs(bank.CreditAmountINR - expectedAmount),
		DateDiffHours: math.Abs(
			bank.ValueDate.
				Sub(settlement.SettledAt).
				Hours(),
		),
		Reason: "exact payment ID, amount and date match",
	}

	addLedgerEvidence(
		&result,
		settlement,
		ledgerByOrder,
	)

	return result
}

func buildFuzzyResult(
	settlement model.Settlement,
	bank model.BankStatement,
	ledgerByOrder map[string]model.LedgerEntry,
) Result {

	expectedAmount :=
		float64(settlement.NetAmountPaisa) / 100

	result := Result{
		OrderID:       settlement.OrderID,
		SettlementID:  settlement.SettlementID,
		BankUTRRef:    bank.UTRRef,
		Tier:          TierFuzzy,
		AmountDiffINR: math.Abs(bank.CreditAmountINR - expectedAmount),
		DateDiffHours: math.Abs(
			bank.ValueDate.
				Sub(settlement.SettledAt).
				Hours(),
		),
		Reason: "fuzzy amount/date match",
	}

	addLedgerEvidence(
		&result,
		settlement,
		ledgerByOrder,
	)

	return result
}

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
		AmountDiffINR: math.Abs(
			bank.CreditAmountINR -
				float64(
					settlement.NetAmountPaisa+
						other.NetAmountPaisa,
				)/100,
		),
		DateDiffHours: math.Abs(
			bank.ValueDate.
				Sub(settlement.SettledAt).
				Hours(),
		),
		Reason: fmt.Sprintf(
			"bank batch matches combined settlement amounts for %s and %s",
			settlement.SettlementID,
			other.SettlementID,
		),
	}

	addLedgerEvidence(
		&result,
		settlement,
		ledgerByOrder,
	)

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

	addLedgerEvidence(
		&result,
		settlement,
		ledgerByOrder,
	)

	return result
}

// ------------------------------------------------------------
// METRICS
// ------------------------------------------------------------

func MatchRate(results []Result) float64 {
	if len(results) == 0 {
		return 0
	}

	matched := 0

	for _, result := range results {
		if result.Tier != TierUnresolved {
			matched++
		}
	}

	return float64(matched) / float64(len(results))
}
