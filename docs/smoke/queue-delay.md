# Queue delay

**Signal:** Three of five unique payments were completed without confirmed merchant callbacks. **Actual result:** `PASS`; model proposed `escalate_internal` to `callback_platform`. The underlying cause remained unconfirmed.

| Step | Executed read-only tool | Returned evidence |
| --- | --- | --- |
| 1 | `get_callback_attempts({"payment_ids":["p1","p2","p3"]})` | `E003`: 0 of 3 payments had attempts; source complete |
| 2 | `get_queue_health({})` | `E004`: depth 845, oldest job 1,080 s, worker degraded |
| 3 | `get_provider_status({})` | `E005`: operational status page, stale for this incident |

All three tool calls succeeded. The model returned:

> Three completed payments are missing confirmed callbacks (E001). No callback attempts were recorded for the affected payments, and the pre-dispatch queue worker was degraded (E003, E004), localizing the customer-visible issue to callback platform. Escalate to callback platform for human review; the underlying cause remains unconfirmed. Provider status is stale (E005).

| Follow-up check | Observed result | Workflow decision |
| --- | --- | --- |
| First | 20/20 new deliveries acknowledged; only 2/3 original payments acknowledged | Closure blocked |
| Second | 20/20 new deliveries and 3/3 original payments acknowledged | Closure proposed; evaluator approved |

The evaluator approved the escalation and closure with synthetic decisions; final local state was `closed`. The explanation matches the tool evidence and does not claim the degraded worker's root cause was confirmed.

**Execution:** 4 API calls, 3 tool calls, 28.55 s, 12,942 input / 2,310 output tokens. Incident trace: `cea703c5-9d0c-4bca-a555-dccdb8e12c54` in the original local database.

### Recorded artifacts

- [Evaluation output](raw/queue-delay/eval.json)
- [Incident state](raw/queue-delay/incident.json)
- [Incident timeline](raw/queue-delay/timeline.json)

[All scenarios](README.md)
