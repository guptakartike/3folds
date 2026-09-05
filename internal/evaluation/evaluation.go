package evaluation

import (
	"fmt"

	"threefolds/internal/matcher"
)

type GroundTruth struct {
	OrderID     string `json:"order_id"`
	Type        string `json:"type"`
	ShouldMatch bool   `json:"should_match"`
	Explanation string `json:"explanation"`
}

type ScenarioSummary struct {
	Total     int
	Correct   int
	Incorrect int
}

type Summary struct {
	Total              int
	Correct            int
	Wrong              int
	Accuracy           float64
	ExpectedMatches    int
	Matched            int
	MatchCoverage      float64
	ExpectedExceptions int
	DetectedExceptions int
	ExceptionDetection float64
	FalsePositives     int
	FalseNegatives     int
	ProcessingTimeMs   float64
	RecordsPerSecond   float64
	ByScenario         map[string]ScenarioSummary
}

func Calculate(
	groundTruth []GroundTruth,
	results []matcher.Result,
) Summary {
	resultByOrder := make(map[string]matcher.Result, len(results))

	for _, result := range results {
		resultByOrder[result.OrderID] = result
	}

	summary := Summary{
		Total:      len(groundTruth),
		ByScenario: make(map[string]ScenarioSummary),
	}

	for _, truth := range groundTruth {
		result, found := resultByOrder[truth.OrderID]

		gotMatch := found && result.Tier != matcher.TierUnresolved

		if truth.ShouldMatch {
			summary.ExpectedMatches++

			if gotMatch {
				summary.Matched++
			} else {
				summary.FalseNegatives++
			}
		} else {
			summary.ExpectedExceptions++

			if gotMatch {
				summary.FalsePositives++
			} else {
				summary.DetectedExceptions++
			}
		}

		correct := gotMatch == truth.ShouldMatch

		if correct {
			summary.Correct++
		} else {
			summary.Wrong++
		}

		scenario := summary.ByScenario[truth.Type]
		scenario.Total++

		if correct {
			scenario.Correct++
		} else {
			scenario.Incorrect++
		}

		summary.ByScenario[truth.Type] = scenario
	}

	if summary.Total > 0 {
		summary.Accuracy = float64(summary.Correct) / float64(summary.Total) * 100
	}

	if summary.ExpectedMatches > 0 {
		summary.MatchCoverage =
			float64(summary.Matched) /
				float64(summary.ExpectedMatches) * 100
	}

	if summary.ExpectedExceptions > 0 {
		summary.ExceptionDetection =
			float64(summary.DetectedExceptions) /
				float64(summary.ExpectedExceptions) * 100
	}

	return summary
}

func Validate(summary Summary) error {
	if summary.Total == 0 {
		return fmt.Errorf("evaluation contains no ground-truth records")
	}

	if summary.Correct+summary.Wrong != summary.Total {
		return fmt.Errorf(
			"evaluation invariant failed: correct=%d wrong=%d total=%d",
			summary.Correct,
			summary.Wrong,
			summary.Total,
		)
	}

	return nil
}
