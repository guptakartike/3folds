// Command 3folds generates synthetic reconciliation data and matches it.
//
// Usage:
//
//	3folds generate -n 60 -seed 42 -out data
//	3folds match -in data
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"threefolds/internal/generator"
	"threefolds/internal/loader"
	"threefolds/internal/matcher"
	"threefolds/internal/report"
	"threefolds/internal/resolver"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("expected a subcommand: generate | match")
	}

	switch os.Args[1] {
	case "generate":
		runGenerate(os.Args[2:])
	case "match":
		runMatch(os.Args[2:])
	case "resolve":
		runResolve(os.Args[2:])
	case "report":
		runReport(os.Args[2:])
	default:
		log.Fatalf("unknown subcommand %q: expected generate | match | resolve | report", os.Args[1])
	}
}

func runGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	n := fs.Int("n", 60, "number of transactions to generate (must be >= 50)")
	seed := fs.Int64("seed", 42, "random seed for reproducibility")
	outDir := fs.String("out", "data", "output directory")
	fs.Parse(args)

	if *n < 50 {
		log.Fatalf("n must be >= 50 to satisfy the track's batch requirement, got %d", *n)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("creating output dir: %v", err)
	}

	ds := generator.Generate(*n, *seed)

	writeJSON(filepath.Join(*outDir, "settlements.json"), ds.Settlements)
	writeJSON(filepath.Join(*outDir, "bank_statements.json"), ds.BankStatements)
	writeJSON(filepath.Join(*outDir, "ledger_entries.json"), ds.LedgerEntries)
	writeJSON(filepath.Join(*outDir, "ground_truth.json"), ds.GroundTruth)

	log.Printf("generated %d transactions -> %s", *n, *outDir)
	log.Printf("  settlements:     %d", len(ds.Settlements))
	log.Printf("  bank_statements: %d (fewer than settlements = genuine exceptions)", len(ds.BankStatements))
	log.Printf("  ledger_entries:  %d", len(ds.LedgerEntries))
}

func runMatch(args []string) {
	fs := flag.NewFlagSet("match", flag.ExitOnError)
	inDir := fs.String("in", "data", "input directory (output of generate)")
	fs.Parse(args)

	d, err := loader.Load(*inDir)
	if err != nil {
		log.Fatalf("loading data: %v", err)
	}

	results := matcher.Match(d.Settlements, d.BankStatements)
	writeJSON(filepath.Join(*inDir, "match_results.json"), results)

	rate := matcher.MatchRate(results)
	counts := map[matcher.Tier]int{}
	for _, r := range results {
		counts[r.Tier]++
	}

	fmt.Printf("\n=== 3folds reconciliation report ===\n")
	fmt.Printf("total settlements:   %d\n", len(results))
	fmt.Printf("match rate:          %.1f%%\n", rate*100)
	fmt.Printf("  exact matches:     %d\n", counts[matcher.TierExact])
	fmt.Printf("  fuzzy matches:     %d\n", counts[matcher.TierFuzzy])
	fmt.Printf("  unresolved:        %d\n", counts[matcher.TierUnresolved])

	if counts[matcher.TierUnresolved] > 0 {
		fmt.Printf("\n--- exceptions ---\n")
		for _, r := range results {
			if r.Tier == matcher.TierUnresolved {
				fmt.Printf("  order=%s settlement=%s reason=%q\n", r.OrderID, r.SettlementID, r.Reason)
			}
		}
	}
	fmt.Printf("\nfull results written to %s\n", filepath.Join(*inDir, "match_results.json"))
}

func runResolve(args []string) {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	inDir := fs.String("in", "data", "input directory (output of generate + match)")
	modelName := fs.String("model", "llama-3.3-70b-versatile", "Groq model id — check your console, this changes over time")
	fs.Parse(args)

	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		log.Fatal("GROQ_API_KEY environment variable not set")
	}

	d, err := loader.Load(*inDir)
	if err != nil {
		log.Fatalf("loading data: %v", err)
	}

	var results []matcher.Result
	readJSON(filepath.Join(*inDir, "match_results.json"), &results)

	settlementLookup := make(map[string]int)
	for i, s := range d.Settlements {
		settlementLookup[s.SettlementID] = i
	}

	candidates := resolver.UnusedBankStatements(d.BankStatements, results)
	client := resolver.NewClient(apiKey, *modelName)

	var resolutions []resolver.Resolution
	upgraded := 0

	for i, r := range results {
		if r.Tier != matcher.TierUnresolved {
			continue
		}
		s := d.Settlements[settlementLookup[r.SettlementID]]

		res, err := client.Resolve(s, candidates)
		if err != nil {
			log.Printf("WARN: resolving %s failed: %v", s.SettlementID, err)
			continue
		}
		resolutions = append(resolutions, res)

		if res.Resolved {
			upgraded++
			results[i].Tier = "llm_resolved"
			results[i].BankUTRRef = res.ProposedMatchUTR
			results[i].Reason = fmt.Sprintf("[%s confidence] %s", res.Confidence, res.Reason)
		} else {
			results[i].Reason = fmt.Sprintf("confirmed exception: %s", res.Reason)
		}
	}

	writeJSON(filepath.Join(*inDir, "resolutions.json"), resolutions)
	writeJSON(filepath.Join(*inDir, "match_results_final.json"), results)

	fmt.Printf("\n=== LLM exception resolver ===\n")
	fmt.Printf("unresolved cases sent to LLM: %d\n", len(resolutions))
	fmt.Printf("upgraded to resolved:         %d\n", upgraded)
	fmt.Printf("confirmed genuine exceptions: %d\n", len(resolutions)-upgraded)
	fmt.Printf("\nfinal results written to %s\n", filepath.Join(*inDir, "match_results_final.json"))
}

func readJSON(path string, v interface{}) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(v); err != nil {
		log.Fatalf("decoding %s: %v", path, err)
	}
}

func runReport(args []string) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	inDir := fs.String("in", "data", "input directory")
	htmlOut := fs.String("html", "data/report.html", "path to write the HTML report")
	fs.Parse(args)

	results, source, err := report.Load(*inDir)
	if err != nil {
		log.Fatalf("loading results: %v", err)
	}
	log.Printf("loaded results from %s", source)

	summary := report.Summarize(results)
	exceptions := report.Exceptions(results)

	report.PrintCLI(summary, exceptions)

	if err := report.WriteHTML(*htmlOut, summary, results, exceptions); err != nil {
		log.Fatalf("writing html report: %v", err)
	}
	fmt.Printf("\nhtml report written to %s\n", *htmlOut)
}

func writeJSON(path string, v interface{}) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("creating %s: %v", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Fatalf("encoding %s: %v", path, err)
	}
}