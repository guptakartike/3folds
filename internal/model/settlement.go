// Package model defines the three data source schemas that the 3folds
// reconciliation agent matches against each other.
package model

import "time"

type Settlement_Status string

// SettlementStatus enumerates the lifecycle states a settlement can be in.

const(
	Settlement_Processed Settlement_Status = "Processed"
	Settlement_Pending Settlement_Status = "Pending"
	Settlement_Reversed Settlement_Status = "Reversed"
	
)
// Settlement is Razorpay's own record of a payout to the merchant.
// All amounts are in paisa (1 INR = 100 paisa) to avoid floating point
// error, matching Razorpay's native representation.

type Settlement struct {
	SettlementID string `json:"settlement_id"` 					// PK, e.g. "stl_Nk8x..."
	PaymentID string `json:"payment_id"`    					// e.g. "pay_Mz3a..."
	OrderID string `json:"order_id"`        					// shared with LedgerEntry.OrderID
	GrossAmountPaisa int64 `json:"gross_amount_paisa"` 			// amount before deductions
	FeePaisa int64 `json:"fee_paisa"`          					// Razorpay's processing fee
	TaxPaisa int64 `json:"tax_paisa"`          					// GST on the fee
	NetAmountPaisa int64 `json:"net_amount_paisa"` 				// gross - fee - tax; what should hit the bank
	SettledAt time.Time `json:"settled_at"`
	Status Settlement_Status `json:"status"`
}