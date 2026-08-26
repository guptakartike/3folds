package model

import "time"

// BankStatement is one line from the merchant's bank account export.
// Amounts are in rupees (float), matching how banks typically export
// statements — this unit mismatch against Settlement is intentional and
// must be normalized before matching.
type BankStatement struct {
	UTRRef          string    `json:"utr_ref"`   // PK, bank's own reference number
	Narration       string    `json:"narration"` // free text; may partially contain PaymentID
	CreditAmountINR float64   `json:"credit_amount_inr"`
	ValueDate       time.Time `json:"value_date"` // when the credit posted
	BatchFlag       bool      `json:"batch_flag"` // true if this line bundles multiple settlements
}