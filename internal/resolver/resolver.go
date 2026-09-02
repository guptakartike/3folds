// Package resolver sends unresolved settlements to an LLM (via the Groq
// API, OpenAI-compatible) so it can propose a match or confirm a genuine
// exception, with a forced JSON shape so the response is always
// parseable.
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

// Resolution is the LLM's verdict on one unresolved settlement.
type Resolution struct {
	OrderID          string `json:"order_id"`
	SettlementID     string `json:"settlement_id"`
	ProposedMatchUTR string `json:"proposed_match_utr,omitempty"` // "" if no match proposed
	Confidence       string `json:"confidence"`                   // "high" | "medium" | "low"
	Resolved         bool   `json:"resolved"`                     // true = LLM proposes a match
	Reason           string `json:"reason"`
}

// Client wraps calls to the Groq chat completions API.
type Client struct {
	APIKey     string
	Model      string // e.g. "llama-3.3-70b-versatile" — confirm current model id in your Groq console
	HTTPClient *http.Client
}

// NewClient builds a resolver client. Pass the model id you have access
// to on Groq; it changes over time so don't hardcode it blindly.
func NewClient(apiKey, model string) *Client {
	return &Client{
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// candidatePrompt is the shape of context sent to the model for one
// unresolved settlement.
type candidatePrompt struct {
	Settlement     model.Settlement      `json:"settlement"`
	NetAmountINR   float64               `json:"net_amount_inr"`
	CandidateBanks []model.BankStatement `json:"candidate_bank_statements"`
}

// groqRequest / groqResponse are the OpenAI-compatible chat completion
// shapes Groq's API uses.
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
	Type string `json:"type"` // "json_object"
}

type groqResponse struct {
	Choices []struct {
		Message groqMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

const systemPrompt = `You are a payment reconciliation assistant. You will be given one
settlement record and a list of candidate bank statement records that did NOT
match it via exact ID lookup or amount/date tolerance rules.

Decide whether any candidate is plausibly the same underlying transaction —
for example, a bank line that bundles this settlement with another (batching),
or a narration that references the transaction indirectly.

Respond with ONLY a JSON object, no other text, in exactly this shape:
{
  "proposed_match_utr": "<utr_ref of the candidate you propose, or empty string if none>",
  "confidence": "high" | "medium" | "low",
  "resolved": true | false,
  "reason": "<one sentence explaining your decision>"
}

If no candidate is plausible, set resolved to false, proposed_match_utr to "",
and explain why in reason (e.g. "no candidate within a reasonable amount or
time window; likely reversed or delayed beyond this batch").`

// Resolve calls the LLM for one unresolved settlement and its remaining
// unused bank candidates.
func (c *Client) Resolve(settlement model.Settlement, candidates []model.BankStatement) (Resolution, error) {
	netINR := float64(settlement.NetAmountPaisa) / 100

	payload, err := json.Marshal(candidatePrompt{
		Settlement:     settlement,
		NetAmountINR:   netINR,
		CandidateBanks: candidates,
	})
	if err != nil {
		return Resolution{}, fmt.Errorf("marshal prompt: %w", err)
	}

	reqBody := groqRequest{
		Model: c.Model,
		Messages: []groqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(payload)},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
		Temperature:    0.1, // low — we want consistent judgment, not creativity
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return Resolution{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, groqEndpoint, bytes.NewReader(body))
	if err != nil {
		return Resolution{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Resolution{}, fmt.Errorf("calling groq: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Resolution{}, fmt.Errorf("reading response: %w", err)
	}

	var gr groqResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return Resolution{}, fmt.Errorf("unmarshal groq response: %w (raw: %s)", err, raw)
	}
	if gr.Error != nil {
		return Resolution{}, fmt.Errorf("groq api error: %s", gr.Error.Message)
	}
	if len(gr.Choices) == 0 {
		return Resolution{}, fmt.Errorf("groq returned no choices (raw: %s)", raw)
	}

	var res Resolution
	if err := json.Unmarshal([]byte(gr.Choices[0].Message.Content), &res); err != nil {
		return Resolution{}, fmt.Errorf("unmarshal model output as JSON: %w (content: %s)", err, gr.Choices[0].Message.Content)
	}
	res.OrderID = settlement.OrderID
	res.SettlementID = settlement.SettlementID

	return res, nil
}

// UnusedBankStatements returns the bank statements that were NOT
// consumed by the exact/fuzzy matcher, i.e. the pool of candidates the
// LLM is allowed to reason over.
func UnusedBankStatements(all []model.BankStatement, results []matcher.Result) []model.BankStatement {
	used := make(map[string]bool)
	for _, r := range results {
		if r.BankUTRRef != "" {
			used[r.BankUTRRef] = true
		}
	}

	var remaining []model.BankStatement
	for _, b := range all {
		if !used[b.UTRRef] {
			remaining = append(remaining, b)
		}
	}
	return remaining
}