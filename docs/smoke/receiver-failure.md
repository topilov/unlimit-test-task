# Receiver failure

**Signal:** Three of five unique payments were completed without confirmed merchant callbacks. **Actual result:** `PASS`; model proposed `escalate_internal` to `merchant_integration`. It localized the receiver-facing failure but did not confirm its underlying cause.

| Step | Executed read-only tool | Returned evidence |
| --- | --- | --- |
| 1 | `get_callback_attempts({"payment_ids":["p1","p2","p3"]})` | `E003`: all 3 had attempts; 4 HTTP 503 responses; 0 acknowledgements |
| 2 | `get_queue_health({})` | `E004`: depth 0, oldest job 0 s, worker healthy |
| 3 | `get_provider_status({})` | `E005`: operational status page, stale for this incident |

All three tool calls succeeded. The model returned:

> Three completed payments are missing confirmed callbacks (E001). Attempts existed for all three, with four HTTP 503 responses and no acknowledgements (E003); the queue was healthy (E004). Escalate to merchant integration for human review of the receiver responses. The failure domain is localized, but the underlying cause is not confirmed; provider status is stale (E005).

| Follow-up check | Observed result | Workflow decision |
| --- | --- | --- |
| First | 20/20 new deliveries acknowledged; only 2/3 original payments acknowledged | Closure blocked |
| Second | 20/20 new deliveries and 3/3 original payments acknowledged | Closure proposed; evaluator approved |

The evaluator approved the escalation and closure with synthetic decisions; final local state was `closed`. The explanation distinguishes three unique affected payments from four failed delivery attempts; Go computed the counts. The cause of the 503 responses remained unconfirmed.

**Execution:** 4 API calls, 3 tool calls, 30.94 s, 13,202 input / 2,864 output tokens. Incident trace: `6d8c1ca7-2169-4ffa-9e36-b862f954a06d` in the original local database.

### Recorded artifacts

- [Evaluation output](raw/receiver-failure/eval.json)
- [Incident state](raw/receiver-failure/incident.json)
- [Incident timeline](raw/receiver-failure/timeline.json)

[All scenarios](README.md)
