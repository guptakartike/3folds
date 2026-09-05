package resolver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"threefolds/internal/matcher"
	"threefolds/internal/model"
)

const groqEndpoint = "https://api.groq.com/openai/v1/chat/completions"

const (
	maxAttempts       = 4
	initialRetryDelay = 1 * time.Second
)

// Resolution is the LLM's verdict on one unresolved settlement.
type Resolution struct {
	OrderID      string   `json:"order_id"`
	SettlementID string   `json:"settlement_id"`
	Decision     string   `json:"decision"` // MATCH | EXCEPTION
	Confidence   float64  `json:"confidence"`
	BankUTRRef   string   `json:"bank_utr_ref,omitempty"`
	Reason       string   `json:"reason"`
	Evidence     []string `json:"evidence"`
}

// Client wraps calls to the Groq OpenAI-compatible API.
type Client struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// NewClient creates a new Groq resolver client.
func NewClient(apiKey, model string) *Client {
	return &Client{
		APIKey: apiKey,
		Model:  model,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type candidatePrompt struct {
	Settlement     model.Settlement      `json:"settlement"`
	NetAmountINR   float64               `json:"net_amount_inr"`
	CandidateBanks []model.BankStatement `json:"candidate_bank_statements"`
}

type groqRequest struct {
	Model          string          `json:"model"`
	Messages       []groqMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Temperature    float64         `json:"temperature"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type groqResponse struct {
	Choices []struct {
		Message groqMessage `json:"message"`
	} `json:"choices"`

	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

const systemPrompt = `
You are a financial reconciliation controller.

Your job is to evaluate whether a Razorpay settlement
can be reconciled against the supplied bank evidence.

RULES:

1. Never invent a transaction, payment ID, UTR, amount, or date.
2. Never assume a missing bank record exists.
3. Do not match based only on semantic similarity.
4. A MATCH requires concrete evidence from the supplied records.
5. Consider:
   - payment ID
   - order ID
   - net settlement amount
   - bank credit amount
   - settlement date
   - bank value date
   - ledger evidence when supplied
6. Fee and tax deductions can explain why gross and net amounts differ.
7. Small rounding differences may be acceptable.
8. Settlement and bank dates may differ because of settlement timing.
9. If evidence is insufficient or contradictory, return EXCEPTION.
10. When uncertain, return EXCEPTION.
11. If returning MATCH, provide the exact UTR from the supplied bank records.
12. Never create or modify a UTR.
13. A MATCH must have at least two independent pieces of supporting evidence.

Return JSON only:

{
  "decision": "MATCH" or "EXCEPTION",
  "confidence": 0.0,
  "bank_utr_ref": "",
  "reason": "",
  "evidence": []
}
`

// Resolve asks the LLM to investigate one unresolved settlement.
func (c *Client) Resolve(
	settlement model.Settlement,
	candidates []model.BankStatement,
) (Resolution, error) {
	netINR := float64(settlement.NetAmountPaisa) / 100

	payload, err := json.Marshal(candidatePrompt{
		Settlement:     settlement,
		NetAmountINR:   netINR,
		CandidateBanks: candidates,
	})
	if err != nil {
		return Resolution{}, fmt.Errorf("marshal resolver prompt: %w", err)
	}

	reqBody := groqRequest{
		Model: c.Model,
		Messages: []groqMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: string(payload),
			},
		},
		ResponseFormat: &responseFormat{
			Type: "json_object",
		},
		Temperature: 0.1,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return Resolution{}, fmt.Errorf("marshal Groq request: %w", err)
	}

	return c.doRequestWithRetry(
		body,
		settlement,
		candidates,
	)
}

// doRequestWithRetry retries transient Groq failures using exponential
// backoff.
//
// Retryable:
//   - HTTP 429
//   - HTTP 500
//   - HTTP 502
//   - HTTP 503
//   - HTTP 504
//   - temporary/network failures
//
// Non-retryable errors such as 400/401 fail immediately.
func (c *Client) doRequestWithRetry(
	body []byte,
	settlement model.Settlement,
	candidates []model.BankStatement,
) (Resolution, error) {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		res, retry, err := c.doRequest(
			body,
			settlement,
			candidates,
		)

		if err == nil {
			return res, nil
		}

		lastErr = err

		if !retry || attempt == maxAttempts {
			break
		}

		delay := initialRetryDelay * time.Duration(1<<(attempt-1))

		time.Sleep(delay)
	}

	return Resolution{}, fmt.Errorf(
		"Groq request failed after %d attempts: %w",
		maxAttempts,
		lastErr,
	)
}

// doRequest performs one Groq API request.
func (c *Client) doRequest(
	body []byte,
	settlement model.Settlement,
	candidates []model.BankStatement,
) (Resolution, bool, error) {
	req, err := http.NewRequest(
		http.MethodPost,
		groqEndpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return Resolution{}, false, fmt.Errorf(
			"build Groq request: %w",
			err,
		)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(
		"Authorization",
		"Bearer "+c.APIKey,
	)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Resolution{}, true, fmt.Errorf(
			"calling Groq: %w",
			err,
		)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Resolution{}, true, fmt.Errorf(
			"reading Groq response: %w",
			err,
		)
	}

	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf(
			"Groq returned HTTP %d",
			resp.StatusCode,
		)

		var gr groqResponse

		if err := json.Unmarshal(raw, &gr); err == nil &&
			gr.Error != nil {
			message = fmt.Sprintf(
				"Groq returned HTTP %d: %s",
				resp.StatusCode,
				gr.Error.Message,
			)
		}

		return Resolution{},
			isRetryableStatus(resp.StatusCode),
			fmt.Errorf("%s", message)
	}

	var gr groqResponse

	if err := json.Unmarshal(raw, &gr); err != nil {
		return Resolution{}, true, fmt.Errorf(
			"unmarshal Groq response: %w",
			err,
		)
	}

	if gr.Error != nil {
		return Resolution{}, false, fmt.Errorf(
			"Groq API error: %s",
			gr.Error.Message,
		)
	}

	if len(gr.Choices) == 0 {
		return Resolution{}, true, fmt.Errorf(
			"Groq returned no choices",
		)
	}

	content := gr.Choices[0].Message.Content

	var resolution Resolution

	if err := json.Unmarshal(
		[]byte(content),
		&resolution,
	); err != nil {
		return Resolution{}, false, fmt.Errorf(
			"unmarshal model output as JSON: %w",
			err,
		)
	}

	// Always use the actual settlement identifiers rather than trusting
	// the model to reproduce them correctly.
	resolution.OrderID = settlement.OrderID
	resolution.SettlementID = settlement.SettlementID

	if err := validateResolution(
		resolution,
		candidates,
	); err != nil {
		return Resolution{}, false, err
	}

	return resolution, false, nil
}

// validateResolution prevents an LLM from turning unsupported reasoning
// into an accepted reconciliation.
func validateResolution(
	res Resolution,
	candidates []model.BankStatement,
) error {
	if res.Decision != "MATCH" &&
		res.Decision != "EXCEPTION" {
		return fmt.Errorf(
			"invalid LLM decision %q",
			res.Decision,
		)
	}

	if res.Confidence < 0 ||
		res.Confidence > 1 {
		return fmt.Errorf(
			"invalid LLM confidence %.3f",
			res.Confidence,
		)
	}

	// Exceptions do not need a bank UTR.
	if res.Decision == "EXCEPTION" {
		return nil
	}

	// A MATCH must satisfy strict evidence requirements.
	if res.Confidence < 0.90 {
		return fmt.Errorf(
			"LLM proposed MATCH with insufficient confidence %.2f",
			res.Confidence,
		)
	}

	if res.BankUTRRef == "" {
		return fmt.Errorf(
			"LLM proposed MATCH without a bank UTR",
		)
	}

	if len(res.Evidence) < 2 {
		return fmt.Errorf(
			"LLM proposed MATCH without sufficient evidence",
		)
	}

	// The model can only select a UTR that was actually supplied
	// in the candidate pool.
	for _, candidate := range candidates {
		if candidate.UTRRef == res.BankUTRRef {
			return nil
		}
	}

	return fmt.Errorf(
		"LLM proposed unknown bank UTR %q",
		res.BankUTRRef,
	)
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true

	default:
		return false
	}
}

// UnusedBankStatements returns bank statements that were not consumed
// by deterministic reconciliation.
func UnusedBankStatements(
	all []model.BankStatement,
	results []matcher.Result,
) []model.BankStatement {
	used := make(map[string]bool)

	for _, r := range results {
		if r.BankUTRRef != "" {
			used[r.BankUTRRef] = true
		}
	}

	remaining := make(
		[]model.BankStatement,
		0,
		len(all),
	)

	for _, bank := range all {
		if !used[bank.UTRRef] {
			remaining = append(
				remaining,
				bank,
			)
		}
	}

	return remaining
}