package matcher

import (
	"testing"

	"threefolds/internal/generator"
)

func BenchmarkMatch(b *testing.B) {
	sizes := []int{
		60,
		600,
		6000,
		60000,
	}

	for _, n := range sizes {
		b.Run(
			recordsLabel(n),
			func(b *testing.B) {
				ds := generator.Generate(n, 42)

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					Match(
						ds.Settlements,
						ds.BankStatements,
						ds.LedgerEntries,
					)
				}
			},
		)
	}
}

func recordsLabel(n int) string {
	switch n {
	case 60:
		return "60"
	case 600:
		return "600"
	case 6000:
		return "6000"
	case 60000:
		return "60000"
	default:
		return "unknown"
	}
}
