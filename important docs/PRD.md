# Product Requirements: 3folds

## Problem

A merchant's money movement is recorded in three places that should agree
and often do not: the payment processor's settlement record, the bank
statement showing what actually landed, and the merchant's own internal
ledger. Reconciling these by hand is slow, and the differences that matter
are rarely simple. They include processor fees, GST, rounding, settlement
timing lag, bundled batch payouts, and genuinely missing or unrelated
transactions. A finance team needs to know, for every transaction, whether
it reconciles, and if it does not, exactly why, backed by evidence rather
than a guess.

## Who this is for

A finance or accounts function at a merchant using a payment processor,
specifically one that needs to close a reconciliation loop across
settlement, bank, and ledger data on a recurring basis, and an auditor or
reviewer who needs to verify that reconciliation decisions were made on
defensible evidence rather than approximated.

## What the product does

1. Accepts three data sources describing the same batch of transactions:
   settlement records, bank statement records, and internal ledger
   records, either generated synthetically for testing or uploaded as real
   CSV or JSON files.
2. Classifies every transaction into one of four outcomes: an exact match,
   a fuzzy match within documented tolerance, a batch match against a
   bundled bank credit, or unresolved.
3. Sends only the unresolved cases to an LLM for a narrow, evidence-gated
   second opinion. The model may only propose a match when it has high
   confidence, a valid candidate reference, and multiple supporting
   details; otherwise the case remains a documented exception.
4. Reports the result at two levels: a summary view for someone who needs
   the headline number, and a full audit trail for someone who needs to
   verify every individual decision and its reasoning.
5. Measures its own output against known ground truth when synthetic data
   is used, so its accuracy claim is a measured result, not an assertion.

## Requirements

### Must have
- Deterministic matching that requires no model call for the majority of
  transactions.
- An LLM review step that is evidence-gated and cannot force a match
  without justification.
- A complete audit trail: every transaction's classification and the
  reason behind it, retrievable after the fact.
- Real file upload for settlement, bank, and ledger data, in addition to
  synthetic data generation for testing.
- A web dashboard that reflects only real backend state: an honest idle
  state before any run, a real loading state during a run, and a populated
  state sourced entirely from the backend afterward. No hardcoded or
  placeholder values anywhere in the interface.
- A way to reset the system back to the idle state without a server
  restart, so the same instance can be run through multiple times.
- A measured accuracy result against ground truth, not only a match rate.

### Should have
- Sample downloadable files for each of the three data sources, so a new
  user has a known-valid file to test with immediately.
- Search and filtering in the review queue and the audit trail, so a human
  reviewer can find a specific transaction quickly.
- Measured throughput and processing time, reported honestly rather than
  rounded in a way that implies false precision.

### Explicitly out of scope for now
- Multi-merchant or multi-tenant support.
- Any claim of cryptographic integrity, consensus, or blockchain-backed
  audit logging. The audit trail is a structured, inspectable log, not a
  distributed ledger, and the product does not claim otherwise anywhere in
  its interface or documentation.
- Automated write-back to a merchant's accounting system. The product
  produces a decision and an audit trail; acting on that decision remains
  a human or downstream-system responsibility.

## Success criteria

- On the canonical 60-record synthetic dataset, the system correctly
  classifies matches and exceptions with measured accuracy, not an
  assumed one, verified by comparing its output against generated ground
  truth.
- Every value shown in the dashboard can be traced to a specific backend
  response field. There are no cases where the interface shows a number
  that does not correspond to something the backend actually computed.
- The LLM review step never converts a genuinely unresolved case into a
  false match; false positives are treated as a more serious failure than
  an unresolved case, since an unresolved case is visible and reviewable
  while a false match is not.
- A user can go from uploading real files to a completed, explained
  reconciliation result without needing to inspect raw JSON or run a
  command line tool.

## Open questions

- What real-world file formats and column layouts should the upload
  parsers support beyond the current CSV and JSON handling, once tested
  against actual processor and bank export formats rather than synthetic
  data.
- Whether the LLM resolver step should support retry with backoff against
  provider rate limits as a first-class behavior, given it is currently a
  single attempt per case.
- Whether reconciliation should ever run incrementally against a growing
  dataset rather than as a full batch each time.
