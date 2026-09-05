package evaluation

import "threefolds/internal/matcher"

type GroundTruth struct {
	OrderID     string `json:"order_id"`
	ShouldMatch bool   `json:"should_match"`
}

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
	ProcessingTimeMs  float64 `json:"processing_time_ms"`
	RecordsPerSecond  float64 `json:"records_per_second"`
}

func Calculate(
	truth []GroundTruth,
	results []matcher.Result,
	processingTimeMs float64,
) Summary {
	s := Summary{
		Total: len(truth),
	}

	resultByOrder := make(map[string]matcher.Result)

	for _, r := range results {
		resultByOrder[r.OrderID] = r
	}

	for _, t := range truth {
		r, exists := resultByOrder[t.OrderID]

		if !exists {
			s.Wrong++

			if t.ShouldMatch {
				s.FalseNegatives++
			}

			continue
		}

		gotMatch := r.Tier != matcher.TierUnresolved

		if gotMatch == t.ShouldMatch {
			s.Correct++
		} else {
			s.Wrong++

			if t.ShouldMatch {
				s.FalseNegatives++
			} else {
				s.FalsePositives++
			}
		}

		if gotMatch {
			s.Matches++
		} else {
			s.Exceptions++
		}
	}

	if s.Total > 0 {
		s.Accuracy = float64(s.Correct) / float64(s.Total) * 100
		s.MatchRate = float64(s.Matches) / float64(s.Total) * 100
		s.ExceptionRate = float64(s.Exceptions) / float64(s.Total) * 100
	}

	s.ProcessingTimeMs = processingTimeMs

	if processingTimeMs > 0 {
		s.RecordsPerSecond =
			float64(s.Total) / (processingTimeMs / 1000)
	}

	return s
}