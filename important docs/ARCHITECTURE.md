# Architecture

## System overview

3folds reconciles three independent records of the same money movement:

```
Razorpay settlement   -> expected cash
Bank statement        -> actual cash
Internal ledger       -> accounting state
```

It produces a per-transaction classification (exact, fuzzy, batch, or
unresolved), escalates only the genuinely ambiguous cases to an LLM, and
reports a measured accuracy and throughput figure rather than a single
unverified match rate.

The system has two independently runnable halves: a Go backend that does
all data processing, and a React frontend that presents it. The frontend
has no logic of its own beyond display and interaction; every number it
shows is fetched from the backend at request time.

## Backend structure

```
cmd/threefolds/main.go       command-line entry point and HTTP server entry point
internal/model/              Settlement, BankStatement, LedgerEntry structs
internal/generator/          synthetic dataset generation with known ground truth
internal/loader/             reads settlement/bank/ledger JSON back into structs
internal/matcher/            exact, fuzzy, and batch matching engine
internal/resolver/           LLM-based exception review (Groq API)
internal/evaluation/         measures matcher output against ground truth
internal/report/             aggregates results into a summary and HTML report
internal/server/             HTTP API serving the same data to the frontend
```

### Matching pipeline

```
Settlements, Bank Statements, Ledger Entries
                |
                v
        Deterministic Matcher
                |
    +-----------+-----------+
    |           |           |
  Exact       Fuzzy       Batch
    |           |           |
    +-----------+-----------+
                |
                v
           Unresolved
                |
                v
      LLM Exception Resolver
                |
         MATCH or EXCEPTION
                |
                v
    Evaluation + Audit Report
```

Matching policy, in order:

1. **Exact.** Payment ID, amount, and date align.
2. **Fuzzy.** Amount or timing differs within an explicit, documented
   tolerance.
3. **Batch.** Multiple real settlements are reconciled against a single
   bundled bank credit.
4. **Unresolved.** No sufficiently supported bank match exists after the
   three deterministic passes.
5. **LLM review.** Only unresolved cases reach the model. It may only
   return a match when it has high confidence, a valid candidate bank
   reference, and more than one piece of supporting evidence. Otherwise
   the case remains an exception. The internal ledger is used as an
   independent validation signal so a bank match is never accepted on
   amount similarity alone.

This ordering is deliberate: the LLM is a narrow, last-resort reviewer for
cases the deterministic logic could not resolve, not the primary matching
mechanism. Its output does not overwrite deterministic results and cannot
force a match without documented evidence.

## Data flow through the CLI

Each pipeline stage is a separate subcommand that reads and writes JSON
files under a data directory, making every stage independently inspectable
and re-runnable:

```
generate  -> settlements.json, bank_statements.json, ledger_entries.json, ground_truth.json
match     -> match_results.json
resolve   -> match_results_final.json, resolutions.json
evaluate  -> evaluation.json
report    -> report.html
verify    -> ground-truth comparison printed to stdout
```

`serve` starts an HTTP server reading from and writing to the same data
directory, so the CLI and the web dashboard operate on identical state.

## Backend HTTP API

All read endpoints return an explicit `has_run` boolean. The frontend uses
this field directly to distinguish a genuine idle state (no reconciliation
has ever completed) from a completed run that happens to have zero
exceptions; it never infers idle state from zero counts.

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/overview` | summary counts, tier breakdown, reconciled value |
| GET | `/api/exceptions` | unresolved items requiring human review |
| GET | `/api/audit-trail` | full per-transaction result set |
| POST | `/api/upload/settlements` | upload a settlement file, CSV or JSON |
| POST | `/api/upload/bank-statements` | upload a bank statement file, CSV or JSON |
| POST | `/api/upload/ledger` | upload a ledger file, CSV or JSON |
| GET | `/api/sample/settlements` | a known-valid sample settlement file |
| GET | `/api/sample/bank-statements` | a known-valid sample bank statement file |
| GET | `/api/sample/ledger` | a known-valid sample ledger file |
| POST | `/api/run` | runs match, resolve, and report against the current uploaded or generated data |
| POST/DELETE | `/api/reset` | clears results and returns the system to the idle state |

## Frontend structure

```
frontend/src/App.tsx                 top-level routing between views
frontend/src/components/
  Navigation.tsx                     top bar; exception count badge sourced from live data
  OverviewView.tsx                   summary, tier distribution, headline match rate
  ExceptionsView.tsx                 review queue for unresolved items
  AuditTrailView.tsx                 full transaction table with filter and search
  UploadView.tsx                     file upload, sample downloads, run trigger, reset
  LoadingState.tsx                   shared loading indicator
  ErrorState.tsx                     shared error display for failed requests
frontend/src/services/api.ts         typed client functions for every backend endpoint
frontend/src/types/api.ts            TypeScript types matching backend JSON exactly
```

Every view has four possible states: idle (no run has occurred), loading
(a request is in flight), error (a request failed), and populated (real
data is present). No view fabricates a value in any of these states; the
idle and error states render explicit messages rather than zeroed-out or
placeholder numbers.

## Why the LLM is scoped narrowly

The architecture deliberately keeps the LLM off the critical path for the
majority of transactions. Exact, fuzzy, and batch matching are pure Go and
require no model call, no network dependency, and no per-transaction cost.
Only the residual unresolved cases, typically a small minority, are sent
for review, and the model's output is still constrained by an evidence
requirement rather than trusted at face value. This keeps the system fast,
inexpensive to run at scale, and auditable: every classification traces to
either a deterministic rule or a specific, logged piece of model reasoning,
never to an unexplained black box decision.
