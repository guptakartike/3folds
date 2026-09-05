// Command 3folds generates synthetic reconciliation data and matches it.
//
// Usage:
//
//	3folds generate -n 60 -seed 42 -out data
//	3folds match -in data
//	3folds resolve -in data
//	3folds evaluate -in data
//	3folds report -in data
package main

import (
	"encoding/json"
	"errors"
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
		log.Fatal("expected a subcommand: generate | match | resolve | report | verify | evaluate")
	}

	var err error

	switch os.Args[1] {
	case "generate":
		err = runGenerate(os.Args[2:])
	case "match":
		err = runMatch(os.Args[2:])
	case "resolve":
		err = runResolve(os.Args[2:])
	case "report":
		err = runReport(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "evaluate":
		err = runEvaluate(os.Args[2:])
	default:
		err = fmt.Errorf(
			"unknown subcommand %q: expected generate | match | resolve | report | verify | evaluate",
			os.Args[1],
		)
	}

	if err != nil {
		log.Fatal(err)
	}
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)

	n := fs.Int("n", 60, "number of transactions to generate (must be >= 50)")
	seed := fs.Int64("seed", 42, "random seed for reproducibility")
	outDir := fs.String("out", "data", "output directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *n < 50 {
		return fmt.Errorf(
			"n must be >= 50 to satisfy the track's batch requirement, got %d",
			*n,
		)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	ds := generator.Generate(*n, *seed)

	writeJSON(filepath.Join(*outDir, "settlements.json"), ds.Settlements)
	writeJSON(filepath.Join(*outDir, "bank_statements.json"), ds.BankStatements)
	writeJSON(filepath.Join(*outDir, "ledger_entries.json"), ds.LedgerEntries)
	writeJSON(filepath.Join(*outDir, "ground_truth.json"), ds.GroundTruth)

	log.Printf("generated %d transactions -> %s", *n, *outDir)
	log.Printf("  settlements:     %d", len(ds.Settlements))
	log.Printf(
		"  bank_statements: %d (fewer than settlements = genuine exceptions)",
		len(ds.BankStatements),
	)
	log.Printf("  ledger_entries:  %d", len(ds.LedgerEntries))

	return nil
}

func runMatch(args []string) error {
	fs := flag.NewFlagSet("match", flag.ContinueOnError)

	inDir := fs.String("in", "data", "input directory (output of generate)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	d, err := loader.Load(*inDir)
	if err != nil {
		return fmt.Errorf("loading data: %w", err)
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

	fmt.Printf("\n=== 3folds reconciliation report ===\n")
	fmt.Printf("total settlements:   %d\n", len(d.Settlements))
	fmt.Printf("match rate:          %.1f%%\n", rate*100)
	fmt.Printf("  exact matches:     %d\n", counts[matcher.TierExact])
	fmt.Printf("  fuzzy matches:     %d\n", counts[matcher.TierFuzzy])
	fmt.Printf("  unresolved:         %d\n", counts[matcher.TierUnresolved])
	fmt.Printf("  batch matches:      %d\n", counts[matcher.TierBatch])
	fmt.Printf("processing time:      %.3f ms\n", processingTimeMs)
	fmt.Printf("throughput:           %.0f records/sec\n", throughput)

	if counts[matcher.TierUnresolved] > 0 {
		fmt.Printf("\n--- exceptions ---\n")

		for _, r := range results {
			if r.Tier != matcher.TierUnresolved {
				continue
			}

			fmt.Printf(
				"  order=%s settlement=%s reason=%q\n",
				r.OrderID,
				r.SettlementID,
				r.Reason,
			)
		}
	}

	fmt.Printf(
		"\nfull results written to %s\n",
		filepath.Join(*inDir, "match_results.json"),
	)

	return nil
}

func runResolve(args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)

	inDir := fs.String(
		"in",
		"data",
		"input directory (output of generate + match)",
	)

	modelName := fs.String(
		"model",
		"openai/gpt-oss-120b",
		"Groq model id",
	)

	if err := fs.Parse(args); err != nil {
		return err
	}

	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		return errors.New("GROQ_API_KEY environment variable not set")
	}

	d, err := loader.Load(*inDir)
	if err != nil {
		return fmt.Errorf("loading data: %w", err)
	}

	var results []matcher.Result
	readJSON(filepath.Join(*inDir, "match_results.json"), &results)

	settlementLookup := make(map[string]int)
	for i, s := range d.Settlements {
		settlementLookup[s.SettlementID] = i
	}

	// Remove any previous final output before starting.
	// This prevents a stale successful result from being mistaken
	// for the result of the current resolve run.
	finalPath := filepath.Join(*inDir, "match_results_final.json")

	if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale final results: %w", err)
	}

	candidatePool := resolver.UnusedBankStatements(
		d.BankStatements,
		results,
	)

	client := resolver.NewClient(apiKey, *modelName)

	var resolutions []resolver.Resolution

	upgraded := 0
	confirmedExceptions := 0
	failed := 0
	toResolve := 0

	for i, r := range results {
		if r.Tier != matcher.TierUnresolved {
			continue
		}

		toResolve++

		settlementIndex, ok := settlementLookup[r.SettlementID]
		if !ok {
			return fmt.Errorf(
				"settlement %s referenced by match result was not found",
				r.SettlementID,
			)
		}

		s := d.Settlements[settlementIndex]

		res, err := client.Resolve(s, candidatePool)
		if err != nil {
			log.Printf(
				"WARN: resolving %s failed: %v",
				s.SettlementID,
				err,
			)

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
					candidatePool = append(
						candidatePool[:j],
						candidatePool[j+1:]...,
					)
					break
				}
			}
		} else {
			confirmedExceptions++

			results[i].Reason = fmt.Sprintf(
				"confirmed exception: %s",
				res.Reason,
			)
		}
	}

	if toResolve > 0 && failed == toResolve {
		return fmt.Errorf(
			"all %d resolve calls failed; final results were not written",
			failed,
		)
	}

	if failed > 0 {
		log.Printf(
			"WARNING: %d of %d resolve calls failed and remain unresolved",
			failed,
			toResolve,
		)
	}

	writeJSON(
		filepath.Join(*inDir, "resolutions.json"),
		resolutions,
	)

	writeJSON(finalPath, results)

	fmt.Printf("\n=== LLM exception resolver ===\n")
	fmt.Printf("unresolved cases sent to LLM: %d\n", toResolve)
	fmt.Printf("successful LLM resolutions:   %d\n", len(resolutions))
	fmt.Printf("upgraded to resolved:         %d\n", upgraded)
	fmt.Printf("confirmed genuine exceptions: %d\n", confirmedExceptions)
	fmt.Printf("failed LLM calls:             %d\n", failed)

	fmt.Printf(
		"\nfinal results written to %s\n",
		finalPath,
	)

	return nil
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

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)

	inDir := fs.String("in", "data", "input directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	results, source, err := report.Load(*inDir)
	if err != nil {
		return fmt.Errorf("loading results: %w", err)
	}

	log.Printf("loaded results from %s", source)

	summary := report.Summarize(results)

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
		return fmt.Errorf("writing HTML report: %w", err)
	}

	log.Printf("html report written to %s", htmlPath)

	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)

	inDir := fs.String("in", "data", "input directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	var truth []generator.GroundTruthEntry
	readJSON(filepath.Join(*inDir, "ground_truth.json"), &truth)

	results, source, err := report.Load(*inDir)
	if err != nil {
		return fmt.Errorf("loading results: %w", err)
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
			wrong = append(
				wrong,
				fmt.Sprintf(
					"order=%s: no result found at all",
					t.OrderID,
				),
			)
			continue
		}

		gotMatch := r.Tier != matcher.TierUnresolved

		if gotMatch == t.ShouldMatch {
			correct++
			continue
		}

		wrong = append(
			wrong,
			fmt.Sprintf(
				"order=%s type=%s: ground truth says should_match=%v, got tier=%s (%s)",
				t.OrderID,
				t.Type,
				t.ShouldMatch,
				r.Tier,
				r.Reason,
			),
		)
	}

	fmt.Printf("\n=== ground truth verification ===\n")
	fmt.Printf("total:   %d\n", len(truth))

	accuracy := float64(0)
	if len(truth) > 0 {
		accuracy = float64(correct) / float64(len(truth)) * 100
	}

	fmt.Printf("correct: %d (%.1f%%)\n", correct, accuracy)
	fmt.Printf("wrong:   %d\n", len(wrong))

	if len(wrong) > 0 {
		fmt.Printf(
			"\n--- discrepancies (worth investigating before the demo) ---\n",
		)

		for _, w := range wrong {
			fmt.Printf("  %s\n", w)
		}
	} else {
		fmt.Printf("\nall results agree with ground truth.\n")
	}

	return nil
}

func runEvaluate(args []string) error {
	fs := flag.NewFlagSet("evaluate", flag.ContinueOnError)

	inDir := fs.String("in", "data", "input directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	truthPath := filepath.Join(*inDir, "ground_truth.json")
	resultsPath := filepath.Join(*inDir, "match_results_final.json")

	// Explicitly require resolve to have completed.
	if _, err := os.Stat(resultsPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(
				"final results not found: %s; run `threefolds resolve -in %s` first",
				resultsPath,
				*inDir,
			)
		}

		return fmt.Errorf("checking final results: %w", err)
	}

	log.Printf("evaluating results from %s", resultsPath)

	truthFile, err := os.ReadFile(truthPath)
	if err != nil {
		return fmt.Errorf("reading ground truth: %w", err)
	}

	var truth []evaluation.GroundTruth

	if err := json.Unmarshal(truthFile, &truth); err != nil {
		return fmt.Errorf("decoding ground truth: %w", err)
	}

	resultsFile, err := os.ReadFile(resultsPath)
	if err != nil {
		return fmt.Errorf("reading final results: %w", err)
	}

	var results []matcher.Result

	if err := json.Unmarshal(resultsFile, &results); err != nil {
		return fmt.Errorf("decoding final results: %w", err)
	}

	// Evaluation itself is intentionally not reported as the reconciliation
	// throughput metric. The actual matcher benchmark is stored in metrics.json.
	summary := evaluation.Calculate(truth, results, 0)

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

	outputPath := filepath.Join(*inDir, "evaluation.json")

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding evaluation: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("writing evaluation: %w", err)
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