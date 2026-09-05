# Stack

## Backend

- **Language:** Go 1.25.5 (module targets this version; builds and passes
  on Go 1.22+ as well).
- **HTTP:** Go standard library `net/http`, using the built-in method-aware
  routing (`mux.HandleFunc("GET /api/overview", ...)`). No third-party web
  framework.
- **LLM provider:** Groq, called through its OpenAI-compatible chat
  completions endpoint over plain HTTP, with JSON-object response
  formatting forced so output is always parseable. No SDK dependency.
- **Storage:** flat JSON files on disk under a data directory
  (`settlements.json`, `bank_statements.json`, `ledger_entries.json`,
  `match_results.json`, `match_results_final.json`, `resolutions.json`,
  `evaluation.json`, `metrics.json`). No database. This is a deliberate
  choice for a batch-processing tool operating on tens of thousands of
  records at most, not a live transactional system.
- **Testing:** the standard `testing` package. Coverage includes matcher
  correctness against a canonical seeded dataset, a matcher throughput
  benchmark scaling from 60 to 60,000 records, evaluation logic against
  ground truth, and server behavior covering the idle and run flow, the
  sample file endpoints, and upload validation.

## Frontend

- **Framework:** React 18, written in TypeScript.
- **Build tool:** Vite 5, with a dev server proxy forwarding `/api/*`
  requests to the Go backend at `http://localhost:8080`, so the frontend
  never needs a hardcoded backend URL in application code.
- **Icons:** `lucide-react`, used in place of emoji or image-based icon
  sets throughout the interface.
- **State and data fetching:** plain React state and effects with a
  hand-written typed API client (`services/api.ts`) and matching type
  definitions (`types/api.ts`); no external state management or data
  fetching library. Reasonable at this scope, where every view fetches
  from a small, fixed set of backend endpoints.
- **Styling:** plain CSS (`index.css`), no CSS framework or component
  library dependency beyond what is listed above.

## Why this stack, specifically

- **Go for the backend** because the core work is data parsing, matching
  logic, and file I/O at potentially large scale, where Go's performance
  and simple concurrency model are a natural fit, and because it produces
  a single static binary with no runtime dependency to install for anyone
  running the CLI independently of the web dashboard.
- **No framework on either side** because the surface area is small and
  fixed: a known set of HTTP endpoints and a known set of views. A
  framework's abstractions would add indirection without solving a
  problem this project actually has.
- **Flat JSON files instead of a database** because every pipeline stage
  already needs to produce an inspectable, diffable artifact for
  verification against ground truth; a database would obscure that rather
  than help it, and the data volume this tool targets does not need one.
- **Groq over a larger hosted model provider for the LLM step** because
  the resolver's job is deliberately narrow, a small number of
  well-specified evidence-gated classifications per run, which does not
  require the largest available model, and Groq's low-latency inference is
  well suited to a step that should not become the bottleneck in an
  otherwise fast, mostly deterministic pipeline.

## Known gaps in the current stack choices

- The Groq resolver client makes a single attempt per unresolved case with
  no retry or backoff on rate-limit responses; a rate-limited call is
  currently logged and left unresolved rather than retried. This is a
  reasonable simplification for a batch of a few dozen cases but would
  need addressing before running against a much larger unresolved set.
- File-based storage does not support concurrent writers safely; the
  server assumes a single active reconciliation run at a time against a
  given data directory.
