package evaluation


import "threefolds/internal/matcher"

type Summary struct {
	Total             int     `json:"total"`
	Correct           int     `json:"correct"`
	Wrong             int     `json:"wrong"`
	Accuracy          float64 `json:"accuracy"`
	Matches           int     `json:"matches"`
	Exceptions        int     `json:"exceptions"`
	MatchRate         float64 `json:"match_rate"`
	ExceptionRate     float64 `json:"exception_rate"`
	FalsePositives    int     `json:"false_positives"`
	FalseNegatives    int     `json:"false_negatives"`
}

func Calculate(
	truth []GroundTruth,
	results []matcher.Result,
) Summary {
	resultByOrder := make(map[string]matcher.Result)

	for _, r := range results {
		resultByOrder[r.OrderID] = r
	}

	var summary Summary
	summary.Total = len(truth)

	for _, t := range truth {
		r, ok := resultByOrder[t.OrderID]
		if !ok {
			summary.Wrong++
			summary.FalseNegatives++
			continue
		}

		gotMatch := r.Tier != matcher.TierUnresolved

		if gotMatch {
			summary.Matches++
		} else {
			summary.Exceptions++
		}

		if gotMatch == t.ShouldMatch {
			summary.Correct++
		} else {
			summary.Wrong++

			if gotMatch {
				summary.FalsePositives++
			} else {
				summary.FalseNegatives++
			}
		}
	}

	if summary.Total > 0 {
		summary.Accuracy = float64(summary.Correct) / float64(summary.Total)
		summary.MatchRate = float64(summary.Matches) / float64(summary.Total)
		summary.ExceptionRate = float64(summary.Exceptions) / float64(summary.Total)
	}

	return summary
}

type GroundTruth struct {
	OrderID    string `json:"order_id"`
	ShouldMatch bool  `json:"should_match"`
}