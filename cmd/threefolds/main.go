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
	"time"

	"threefolds/internal/evaluation"
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
	case "verify":
		runVerify(os.Args[2:])
	case "evaluate":
		runEvaluate(os.Args[2:])
	default:
		log.Fatalf("unknown subcommand %q: expected generate | match | resolve | report | verify | evaluate", os.Args[1])
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

	// Measure the actual reconciliation operation.
	start := time.Now()

	results := matcher.Match(
		d.Settlements,
		d.BankStatements,
		d.LedgerEntries,
	)

	elapsed := time.Since(start)
	processingTimeMs := float64(elapsed.Nanoseconds()) / 1e6

	throughput := float64(0)

	if processingTimeMs > 0 {
		throughput = float64(len(d.Settlements)) / (processingTimeMs / 1000)
	}

	metrics := map[string]float64{
		"processing_time_ms": processingTimeMs,
		"records_per_second": throughput,
	}

	writeJSON(filepath.Join(*inDir, "metrics.json"), metrics)

	writeJSON(filepath.Join(*inDir, "match_results.json"), results)

	rate := matcher.MatchRate(results)
	counts := map[matcher.Tier]int{}
	for _, r := range results {
		counts[r.Tier]++
	}

	throughput = 0.0
	if processingTimeMs > 0 {
		throughput = float64(len(d.Settlements)) / (processingTimeMs / 1000)
	}

	fmt.Printf("\n=== 3folds reconciliation report ===\n")
	fmt.Printf("total settlements:   %d\n", len(d.Settlements))
	fmt.Printf("match rate:          %.1f%%\n", rate*100)
	fmt.Printf("  exact matches:     %d\n", counts[matcher.TierExact])
	fmt.Printf("  fuzzy matches:     %d\n", counts[matcher.TierFuzzy])
	fmt.Printf("  unresolved:        %d\n", counts[matcher.TierUnresolved])
	fmt.Printf("  batch matches:     %d\n", counts[matcher.TierBatch])
	fmt.Printf("processing time:     %.3f ms\n", processingTimeMs)
	fmt.Printf("throughput:          %.0f records/sec\n", throughput)

	if counts[matcher.TierUnresolved] > 0 {
		fmt.Printf("\n--- exceptions ---\n")
		for _, r := range results {
			if r.Tier == matcher.TierUnresolved {
				fmt.Printf(
					"  order=%s settlement=%s reason=%q\n",
					r.OrderID,
					r.SettlementID,
					r.Reason,
				)
			}
		}
	}

	fmt.Printf(
		"\nfull results written to %s\n",
		filepath.Join(*inDir, "match_results.json"),
	)
}

func runResolve(args []string) {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	inDir := fs.String("in", "data", "input directory (output of generate + match)")
	modelName := fs.String("model", "openai/gpt-oss-120b", "Groq model id — check console.groq.com/docs/models if this stops working, Groq deprecates models over time")
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

	// A mutable pool: once a candidate is proposed as a match by the LLM,
	// remove it so a later settlement in this same loop can't also claim
	// it — the loop is sequential, so this is safe without locking.
	candidatePool := resolver.UnusedBankStatements(d.BankStatements, results)
	client := resolver.NewClient(apiKey, *modelName)

	var resolutions []resolver.Resolution
	upgraded := 0
	failed := 0
	toResolve := 0

	for i, r := range results {
		if r.Tier != matcher.TierUnresolved {
			continue
		}
		toResolve++
		s := d.Settlements[settlementLookup[r.SettlementID]]

		res, err := client.Resolve(s, candidatePool)
		if err != nil {
			log.Printf("WARN: resolving %s failed: %v", s.SettlementID, err)
			failed++
			continue
		}
		resolutions = append(resolutions, res)

		if res.Decision == "MATCH" {
			upgraded++
			results[i].Tier = "llm_resolved"
			results[i].BankUTRRef = res.BankUTRRef
			results[i].Reason = fmt.Sprintf(
				"[LLM confidence %.2f] %s",
				res.Confidence,
				res.Reason,
			)

			// Remove the claimed bank candidate so it cannot be reused.
			for j, c := range candidatePool {
				if c.UTRRef == res.BankUTRRef {
					candidatePool = append(candidatePool[:j], candidatePool[j+1:]...)
					break
				}
			}
		} else {
			results[i].Reason = fmt.Sprintf(
				"confirmed exception: %s",
				res.Reason,
			)
		}
	}

	// If every single call failed (e.g. a bad model id or bad API key),
	// don't silently write an empty result set that looks like a clean
	// run with zero LLM-resolvable exceptions — fail loudly instead.
	if toResolve > 0 && failed == toResolve {
		log.Fatalf("all %d resolve calls failed — check GROQ_API_KEY and -model (see console.groq.com/docs/models); NOT writing results", failed)
	}
	if failed > 0 {
		log.Printf("WARNING: %d of %d resolve calls failed and were left unresolved — check the errors above", failed, toResolve)
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
	fs.Parse(args)

	results, source, err := report.Load(*inDir)
	if err != nil {
		log.Fatalf("loading results: %v", err)
	}

	log.Printf("loaded results from %s", source)

	summary := report.Summarize(results)

	// Load measured throughput from evaluation.json.
	metricsPath := filepath.Join(*inDir, "metrics.json")

	if data, err := os.ReadFile(metricsPath); err == nil {
		var metrics struct {
			ProcessingTimeMs float64 `json:"processing_time_ms"`
			RecordsPerSecond float64 `json:"records_per_second"`
		}

		if err := json.Unmarshal(data, &metrics); err == nil {
			summary = report.SetThroughput(
				summary,
				metrics.ProcessingTimeMs,
			)
		}
	}

	exceptions := report.Exceptions(results)

	report.PrintCLI(summary, exceptions)

	htmlPath := filepath.Join(*inDir, "report.html")

	if err := report.WriteHTML(
		htmlPath,
		summary,
		results,
		exceptions,
	); err != nil {
		log.Fatalf("writing HTML report: %v", err)
	}

	log.Printf("html report written to %s", htmlPath)
}

func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	inDir := fs.String("in", "data", "input directory")
	fs.Parse(args)

	var truth []generator.GroundTruthEntry
	readJSON(filepath.Join(*inDir, "ground_truth.json"), &truth)

	results, source, err := report.Load(*inDir)
	if err != nil {
		log.Fatalf("loading results: %v", err)
	}
	log.Printf("verifying against %s", source)

	resultByOrder := make(map[string]matcher.Result)
	for _, r := range results {
		resultByOrder[r.OrderID] = r
	}

	var wrong []string
	correct := 0

	for _, t := range truth {
		r, ok := resultByOrder[t.OrderID]
		if !ok {
			wrong = append(wrong, fmt.Sprintf("order=%s: no result found at all", t.OrderID))
			continue
		}
		gotMatch := r.Tier != matcher.TierUnresolved
		if gotMatch == t.ShouldMatch {
			correct++
			continue
		}
		wrong = append(wrong, fmt.Sprintf(
			"order=%s type=%s: ground truth says should_match=%v, got tier=%s (%s)",
			t.OrderID, t.Type, t.ShouldMatch, r.Tier, r.Reason,
		))
	}

	fmt.Printf("\n=== ground truth verification ===\n")
	fmt.Printf("total:   %d\n", len(truth))
	fmt.Printf("correct: %d (%.1f%%)\n", correct, float64(correct)/float64(len(truth))*100)
	fmt.Printf("wrong:   %d\n", len(wrong))

	if len(wrong) > 0 {
		fmt.Printf("\n--- discrepancies (worth investigating before the demo) ---\n")
		for _, w := range wrong {
			fmt.Printf("  %s\n", w)
		}
	} else {
		fmt.Printf("\nall results agree with ground truth.\n")
	}
}

func runEvaluate(args []string) error {
	fs := flag.NewFlagSet("evaluate", flag.ExitOnError)

	inDir := fs.String("in", "data", "input directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	truthPath := filepath.Join(*inDir, "ground_truth.json")
	resultsPath := filepath.Join(*inDir, "match_results_final.json")

	log.Printf("evaluating results from %s", resultsPath)

	truthFile, err := os.ReadFile(truthPath)
	if err != nil {
		return err
	}

	var truth []evaluation.GroundTruth
	if err := json.Unmarshal(truthFile, &truth); err != nil {
		return err
	}

	resultsFile, err := os.ReadFile(resultsPath)
	if err != nil {
		return err
	}

	var results []matcher.Result
	if err := json.Unmarshal(resultsFile, &results); err != nil {
		return err
	}

	start := time.Now()

	summary := evaluation.Calculate(truth, results, 0)

	elapsed := time.Since(start)
	processingTimeMs := float64(elapsed.Nanoseconds()) / 1e6

	summary = evaluation.Calculate(truth, results, processingTimeMs)

	fmt.Println()
	fmt.Println("=== evaluation ===")
	fmt.Printf("total:             %d\n", summary.Total)
	fmt.Printf("correct:           %d\n", summary.Correct)
	fmt.Printf("wrong:             %d\n", summary.Wrong)
	fmt.Printf("accuracy:          %.1f%%\n", summary.Accuracy)
	fmt.Printf("match rate:        %.1f%%\n", summary.MatchRate)
	fmt.Printf("exception rate:    %.1f%%\n", summary.ExceptionRate)
	fmt.Printf("false positives:   %d\n", summary.FalsePositives)
	fmt.Printf("false negatives:   %d\n", summary.FalseNegatives)
	fmt.Printf("processing time:   %.3f ms\n", summary.ProcessingTimeMs)
	fmt.Printf("throughput:        %.0f records/sec\n", summary.RecordsPerSecond)

	outputPath := filepath.Join(*inDir, "evaluation.json")

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return err
	}

	log.Printf("evaluation written to %s", outputPath)

	return nil
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
