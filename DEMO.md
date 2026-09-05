# 3folds — Demo Script

## 90-second flow

### 1. Establish the problem

> Finance teams reconcile expected settlements against actual bank cash and internal ledger records. Exact matches are easy; the operational risk is in differences, batches, timing gaps, and missing transactions.

### 2. Show the deterministic pass

```bash
go run ./cmd/threefolds generate -n 60 -seed 42 -out data
go run ./cmd/threefolds match -in data
```

Point out:

- 60 settlements processed
- 42 exact matches
- 9 fuzzy matches
- 6 batch matches
- 3 unresolved
- measured processing time and throughput

### 3. Show controlled AI escalation

```bash
go run ./cmd/threefolds resolve -in data
go run ./cmd/threefolds evaluate -in data
go run ./cmd/threefolds report -in data
open data/report.html
```

Explain:

> Only the 3 unresolved cases go to the LLM. The model is not allowed to invent a match. In this run it confirmed all 3 as genuine exceptions.

### 4. Show the evidence

The report should make these numbers visible:

- **95.0% automatic reconciliation**
- **100.0% match / exception classification accuracy**
- **57/57 expected matches found**
- **3/3 genuine exceptions detected**
- **0 false positives**
- **0 false negatives**

Then open the **honest exception queue** and show the full audit trail.

## Closing line

> 3folds does not optimize for saying MATCH. It optimizes for making a finance decision only when the evidence supports it, measuring the result, and preserving everything else as an auditable exception.
