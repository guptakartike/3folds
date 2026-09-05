# 3folds — AI Finance Controller

3folds is an evidence-first finance reconciliation controller that
reconciles settlement records against bank statements and internal
ledger data.

It uses deterministic matching for high-confidence cases and escalates
unresolved cases to an LLM instead of forcing unsupported matches.

## Problem

Finance operations teams still spend significant time reconciling:

- Expected settlements
- Actual bank credits
- Internal accounting records

The difficult cases are not the obvious matches. They are:

- Amount differences
- Settlement timing differences
- Batched payouts
- Missing transactions
- Ambiguous records

3folds automates the reconciliation loop while maintaining an explicit
exception queue for cases that cannot be resolved with sufficient
evidence.

## Architecture

```text
Synthetic Data
      |
      v
+---------------------------+
| Settlement + Bank + Ledger|
+---------------------------+
              |
              v
+---------------------------+
| Deterministic Matcher     |
|                           |
| Exact                     |
| Batch                     |
| Fuzzy                     |
| Unresolved                |
+---------------------------+
              |
              v
      Unresolved Cases
              |
              v
+---------------------------+
| LLM Exception Resolver    |
|                           |
| MATCH or EXCEPTION        |
| Confidence + Evidence     |
+---------------------------+
              |
              v
+---------------------------+
| Evaluation + Audit Report |
+---------------------------+