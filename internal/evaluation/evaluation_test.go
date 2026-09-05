package evaluation

import (
	"testing"

	"threefolds/internal/generator"
	"threefolds/internal/matcher"
)

func TestCalculateCanonicalDataset(t *testing.T) {
	data := generator.Generate(60, 42)
	results := matcher.Match(data.Settlements, data.BankStatements, data.LedgerEntries)

	truth := make([]GroundTruth, 0, len(data.GroundTruth))
	for _, entry := range data.GroundTruth {
		truth = append(truth, GroundTruth{
			OrderID:     entry.OrderID,
			Type:        string(entry.Type),
			ShouldMatch: entry.ShouldMatch,
			Explanation: entry.Explanation,
		})
	}

	summary := Calculate(truth, results, 0)

	if err := Validate(summary); err != nil {
		t.Fatalf("canonical evaluation should validate: %v", err)
	}
	if summary.Accuracy != 100 {
		t.Errorf("expected 100%% classification accuracy, got %.1f%%", summary.Accuracy)
	}
	if summary.ExpectedMatches != 57 || summary.Matched != 57 {
		t.Errorf("expected 57/57 matches, got %d/%d", summary.Matched, summary.ExpectedMatches)
	}
	if summary.ExpectedExceptions != 3 || summary.DetectedExceptions != 3 {
		t.Errorf("expected 3/3 exceptions, got %d/%d", summary.DetectedExceptions, summary.ExpectedExceptions)
	}
	if summary.FalsePositives != 0 || summary.FalseNegatives != 0 {
		t.Errorf("expected zero FP/FN, got FP=%d FN=%d", summary.FalsePositives, summary.FalseNegatives)
	}
}
