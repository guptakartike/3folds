package matcher

import (
	"testing"

	"threefolds/internal/generator"
)

func TestMatchCanonicalDataset(t *testing.T) {
	data := generator.Generate(60, 42)

	results := Match(
		data.Settlements,
		data.BankStatements,
		data.LedgerEntries,
	)

	if len(results) != 60 {
		t.Fatalf("expected 60 results, got %d", len(results))
	}

	var exact, fuzzy, batch, unresolved int

	for _, result := range results {
		switch result.Tier {
		case TierExact:
			exact++
		case TierFuzzy:
			fuzzy++
		case TierBatch:
			batch++
		case TierUnresolved:
			unresolved++
		default:
			t.Fatalf("unexpected tier %q for settlement %s", result.Tier, result.SettlementID)
		}
	}

	if exact != 42 {
		t.Errorf("expected 42 exact matches, got %d", exact)
	}
	if fuzzy != 9 {
		t.Errorf("expected 9 fuzzy matches, got %d", fuzzy)
	}
	if batch != 6 {
		t.Errorf("expected 6 batch matches, got %d", batch)
	}
	if unresolved != 3 {
		t.Errorf("expected 3 unresolved matches, got %d", unresolved)
	}

	rate := MatchRate(results)
	if rate != 0.95 {
		t.Errorf("expected 95%% match rate, got %.2f%%", rate*100)
	}
}
