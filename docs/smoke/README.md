# Live API integration checks

Recorded on 22 September 2026 at 23:21-23:24 UTC, before the final source-validation, routing and CLI fixes. These are historical results, not a live re-verification of those changes.

Real `gpt-6-luna` API calls over four synthetic payment incidents. The CLI queried saved diagnostics; it did not contact a PSP or merchant. The evaluator supplied synthetic approval decisions. **4/4 scenario workflow checks passed.**

| Scenario | What the model did | Result |
| --- | --- | --- |
| [Queue delay](queue-delay.md) | Found missing callback attempts and a degraded queue | Escalated to `callback_platform`; closure after recovery checks |
| [Receiver failure](receiver-failure.md) | Found HTTP 503 responses with a healthy queue | Escalated to `merchant_integration`; closure after recovery checks |
| [Insufficient evidence](insufficient-evidence.md) | Declined to localize an unproven failure | `manual_triage`; no escalation proposal |
| [Evolving evidence](evolving-evidence.md) | Revised the diagnosis and owner when new data arrived | `callback_platform` → `merchant_integration` |

**Run settings:** `gpt-6-luna`, reasoning `high`, 6 model turns per investigation, 4,000 maximum output tokens per response, 45-second turn timeout, `store=false`, `parallel_tool_calls=false`, approved memory disabled. Total: **21 API calls, 16 successful tool executions, 77,140 input tokens, 13,622 output tokens, 156.68 seconds** including local workflow checks. No API retries.

**Commands used:**

```sh
make db-up migrate build
./bin/apm eval --scenario queue-delay --ai-mode live --without-memory --json
./bin/apm eval --scenario receiver-failure --ai-mode live --without-memory --json
./bin/apm eval --scenario insufficient-evidence --ai-mode live --without-memory --json
./bin/apm eval --scenario evolving-evidence --ai-mode live --without-memory --json
```

I inspected each case with `./bin/apm incident show <id> --json` and `./bin/apm incident timeline <id> --json`. The key came from ignored local `.env`; its value is absent from these files.

`PASS` covers structured contracts and workflow; the evaluator reports `prose_evaluated=false`. Free-form model explanations still require human review. These runs do not measure accuracy or operational time savings. Closure checks confirm callback acknowledgements, not merchant order processing.
