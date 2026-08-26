package model

import "time"

// LedgerEntry is the merchant's internal bookkeeping record, written
// independently of Razorpay/bank timing — expected dates can legitimately
// diverge from actual settlement dates.
type LedgerEntry struct {
	OrderID                string    `json:"order_id"` // PK, shared with Settlement.OrderID
	CustomerID             string    `json:"customer_id"`
	GrossAmountINR         float64   `json:"gross_amount_inr"`
	ExpectedSettlementDate time.Time `json:"expected_settlement_date"`
	InternalStatus         string    `json:"internal_status"` // "paid" | "refunded" | "pending"
}