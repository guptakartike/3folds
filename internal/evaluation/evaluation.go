package evaluation

import (
	"fmt"

	"threefolds/internal/matcher"
)

// GroundTruth describes the expected outcome for one settlement.
type GroundTruth struct {
	OrderID     string `json:"order_id"`
	Type        string `json:"type"`
	ShouldMatch bool   `json:"should_match"`
	Explanation string `json:"explanation"`
}

// ScenarioSummary records classification performance for one synthetic scenario.
type ScenarioSummary struct {
	Total     int `json:"total"`
	Correct   int `json:"correct"`
	Incorrect int `json:"incorrect"`
}

// Summary contains the measured classification and exception metrics required
// by the finance-controller track.
type Summary struct {
	Total              int                        `json:"total"`
	Correct            int                        `json:"correct"`
	Wrong              int                        `json:"wrong"`
	Accuracy           float64                    `json:"accuracy"`
	ExpectedMatches    int                        `json:"expected_matches"`
	Matched            int                        `json:"matched"`
	MatchCoverage      float64                    `json:"match_coverage"`
	ExpectedExceptions int                        `json:"expected_exceptions"`
	DetectedExceptions int                        `json:"detected_exceptions"`
	ExceptionDetection float64                    `json:"exception_detection"`
	FalsePositives     int                        `json:"false_positives"`
	FalseNegatives     int                        `json:"false_negatives"`
	ProcessingTimeMs   float64                    `json:"processing_time_ms"`
	RecordsPerSecond   float64                    `json:"records_per_second"`
	ByScenario         map[string]ScenarioSummary `json:"by_scenario"`
}

// Calculate compares final results against ground truth.
func Calculate(
	groundTruth []GroundTruth,
	results []matcher.Result,
	processingTimeMs float64,
) Summary {
	s := Summary{
		Total:            len(groundTruth),
		ProcessingTimeMs: processingTimeMs,
		ByScenario:       make(map[string]ScenarioSummary),
	}

	resultByOrder := make(map[string]matcher.Result, len(results))
	for _, result := range results {
		resultByOrder[result.OrderID] = result
	}

	for _, truth := range groundTruth {
		if truth.ShouldMatch {
			s.ExpectedMatches++
		} else {
			s.ExpectedExceptions++
		}

		result, found := resultByOrder[truth.OrderID]
		gotMatch := found && result.Tier != matcher.TierUnresolved

		if gotMatch {
			s.Matched++
		} else if found {
			s.DetectedExceptions++
		}

		correct := found && gotMatch == truth.ShouldMatch
		if correct {
			s.Correct++
		} else {
			s.Wrong++
		}

		if truth.ShouldMatch && !gotMatch {
			s.FalseNegatives++
		}
		if !truth.ShouldMatch && gotMatch {
			s.FalsePositives++
		}

		scenario := s.ByScenario[truth.Type]
		scenario.Total++
		if correct {
			scenario.Correct++
		} else {
			scenario.Incorrect++
		}
		s.ByScenario[truth.Type] = scenario
	}

	if s.Total > 0 {
		s.Accuracy = float64(s.Correct) / float64(s.Total) * 100
	}

	if s.ExpectedMatches > 0 {
		s.MatchCoverage = float64(s.Matched) / float64(s.ExpectedMatches) * 100
	}

	if s.ExpectedExceptions > 0 {
		s.ExceptionDetection =
			float64(s.DetectedExceptions) / float64(s.ExpectedExceptions) * 100
	} else {
		s.ExceptionDetection = 100
	}

	if processingTimeMs > 0 {
		s.RecordsPerSecond =
			float64(s.Total) / (processingTimeMs / 1000)
	}

	return s
}

// Validate fails when the evaluation contains missing or incorrect results.
func Validate(summary Summary) error {
	if summary.Total == 0 {
		return fmt.Errorf("ground truth is empty")
	}
	if summary.Wrong != 0 {
		return fmt.Errorf("evaluation found %d incorrect classifications", summary.Wrong)
	}
	if summary.FalsePositives != 0 || summary.FalseNegatives != 0 {
		return fmt.Errorf(
			"evaluation found false positives=%d false negatives=%d",
			summary.FalsePositives,
			summary.FalseNegatives,
		)
	}
	return nil
}
