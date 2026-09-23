# Evolving evidence

**Signal:** Three completed payments lacked confirmed merchant callbacks. **Actual result:** `PASS` across two investigation passes; new evidence changed the recommended owner. The first proposal stayed pending; the evaluator approved the later escalation and closure.

| Snapshot | Actual read-only calls, in order | Evidence and model output |
| --- | --- | --- |
| Initial | `get_callback_attempts` → `get_queue_health` → `get_provider_status` | `E003`: no attempts for 3 payments; `E004`: queue degraded (depth 845, oldest 1,080 s); `E005`: PSP status stale. Proposed `callback_platform`. |
| New evidence | `get_callback_attempts` → `get_queue_health` → `get_provider_status` | `E008`: 503 responses for all 3 payments, no acknowledgements; `E009`: queue now healthy; `E010`: PSP status stale. Proposed `merchant_integration`. |

Each callback call used `{"payment_ids":["p1","p2","p3"]}`; queue and provider calls used `{}`. All six tool calls succeeded. The first model summary was:

> Three completed payments are missing confirmed callbacks (E001). No callback attempts were recorded for the checked cohort, and the pre-dispatch queue was degraded (E003, E004), supporting escalation to the callback platform team for human review. The specific cause is not confirmed. Provider status is stale and does not establish current health (E005).

With new evidence, the model returned:

> Three completed payments are affected; callback attempts for all three returned 503 with no acknowledgements (E006, E008). The later queue check is healthy despite earlier degradation (E004, E009); provider status is stale (E010). Escalate the receiver-facing failure domain for human review; the cause of the 503s remains unknown.

The model weakened its earlier queue hypothesis rather than treating the first snapshot as current. Its final flat `claims` list includes both historical `queue_degraded` and current `queue_healthy`; the explanation distinguishes their observation times. Read the current conclusion from the later investigation result in `incident.json`, not from the full historical claims list.

| Follow-up check | Observed result | Workflow decision |
| --- | --- | --- |
| First | 20/20 new deliveries acknowledged; only 2/3 original payments acknowledged | Closure blocked |
| Second | 20/20 new deliveries and 3/3 original payments acknowledged | Closure proposed; evaluator approved |

Final local state was `closed` after synthetic evaluator decisions. The underlying reason for the 503 responses remained unconfirmed.

**Execution:** 8 API calls across 2 investigations, 6 tool calls, 69.69 s, 33,426 input / 6,031 output tokens. Incident trace: `8cccae08-2c49-4ad1-806a-9ac8a71e10b1` in the original local database.

### Recorded artifacts

- [Evaluation output](raw/evolving-evidence/eval.json)
- [Incident state](raw/evolving-evidence/incident.json)
- [Incident timeline](raw/evolving-evidence/timeline.json)

[All scenarios](README.md)
