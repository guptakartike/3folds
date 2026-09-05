package model

import "time"

// BankStatement is one line from the merchant's bank account export.
type BankStatement struct {
	UTRRef          string    `json:"utr_ref"`
	Narration       string    `json:"narration"`
	CreditAmountINR float64   `json:"credit_amount_inr"`
	ValueDate       time.Time `json:"value_date"`
	BatchFlag       bool      `json:"batch_flag"`

	// BatchID identifies a bank credit that represents multiple settlements.
	BatchID string `json:"batch_id,omitempty"`
}