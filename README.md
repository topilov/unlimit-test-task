# APM Incident Investigator

A Go/PostgreSQL CLI prototype for investigating synthetic APM payment incidents. It implements incident triage, operator-reviewed escalation and callback recovery, plus diagnostic guidance from reviewed feedback. Handoff/status-message drafting is a proposed extension.

## Run and verify

Requires Go 1.25.14, Docker Compose, Python 3, and `make`. From the repository root:

```sh
make deps
make verify
make demo
make eval
```

`make deps` installs the pinned `sqlc` generator. `make verify` checks generated queries, formatting, Go tests, PostgreSQL integration tests, four mock scenarios, and CLI/feedback flows. The mock path needs no API key.

`make demo` starts local PostgreSQL on port 55432, migrates the database, builds `./bin/apm`, and investigates `queue-delay`. Follow the printed `Next:` command, review the proposal, then approve or reject it. Subsequent commands guide recovery and closure. `make eval` runs all four mock scenarios. `make db-down` stops PostgreSQL and keeps its data volume.

Printed commands use the launched executable and carry the selected mode, custom scenario directory and `--without-memory` through review and continuation. A standalone proposal view recovers live/mock mode from the latest investigation; repeat custom scenario/memory flags if starting a separate command chain. Commands assume the same repository working directory and database environment.

## Optional live model mode

Export `OPENAI_API_KEY`, then run `make demo-live` or `make eval-live`. Settings and defaults are in [.env.example](.env.example); the CLI does not load `.env` automatically. Never commit a key.

Four historical runs exercised real `gpt-6-luna` calls over synthetic diagnostics. [Recorded live API integration checks](docs/smoke/README.md) include settings, commands, tools and outcomes. They predate the final validation fixes and do not measure diagnostic accuracy or time savings.

## Controls and limits

Go restricts diagnostic scope and read-only access, validates evidence references, derives severity, and requires human approval for escalation and closure. Targeted routing requires a cited, supported finding from the latest relevant available diagnostic within five minutes: queue degradation for `callback_platform`, or receiver HTTP 503 for `merchant_integration`. Other technical owners have no positive localization rule in this prototype; insufficient localization stays with TechOps. Unknown root cause does not by itself prevent routing.

Historical evidence remains in the timeline and investigation context. It can explain earlier behavior; current statements need current observations or an explicit historical qualification. Claim labels and references do not establish the correctness of free-form prose. Operators still review the explanation and owner. Approval is rejected after a newer incident revision supersedes the proposal.

Recovery separately requires fresh, complete callback data, acknowledgements for the original cohort, and healthy new traffic. Malformed callbacks and conflicting latest payment records cannot establish a complete source. Callback acknowledgement does not prove merchant-side order reconciliation.

All source data and ticket actions are simulated. There are no real PSP, merchant, Slack or ticket-system integrations, and no production operator authentication. The model cannot retry/refund payments or change configuration. Jev is only an optional experiment described in the proposal.

Run `make feedback-demo` to see a correction become a proposed diagnostic rule. A human must approve it before later investigations can use it, and it never replaces current evidence.
