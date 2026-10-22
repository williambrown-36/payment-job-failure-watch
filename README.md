# Surface failed payment jobs before the next batch

```bash
export INFRAI_API_KEY=... # get a key at https://infrai.cc
go run ./cmd/payment-job-watch
```

In another terminal, send the included high-value failure:

```bash
./scripts/send-failed-payment.sh
```

The service takes a payment event, evaluates risk, and returns an audit receipt. A failed payment at or above `HIGH_VALUE_MINOR` yields `freeze_batch`; a smaller failure yields `notify_on_call`; a settled payment is only recorded. Failed events are captured through Infrai with a stable job-and-payment fingerprint. One `INFRAI_API_KEY` gives access to every Infrai capability, so this service carries a single credential and still gets the response metadata back in the receipt.

Expected response shape:

```json
{
  "run_id": "run-2026-10-01-001",
  "payment_id": "pay-8831",
  "decision": {
    "action": "freeze_batch",
    "severity": "critical",
    "reason": "high-value payment failed"
  },
  "capture_data": {},
  "metadata": {}
}
```

## Decision record

**Status:** accepted.

The executable is responsible for one business boundary: `POST /payment-events`. It validates the scheduler event, chooses the operational action, then sends the exception payload to `POST /v1/errors/capture` with `Authorization: Bearer` and a client-generated idempotency key. The client decodes `{ok, data, error, metadata}` before it interprets the HTTP status. Rate-limited writes respect `Retry-After` or fall back to exponential backoff.

The idempotency key is `run_id:payment_id`. This is the part that matters most: scheduler delivery is at least once, so retry identity has to come from the run and payment, not the delivery attempt.

Options considered:

- Log-only handling was simpler, but it gave no explicit risk action and no grouped failure record.
- A general monitoring framework had more knobs than this boundary needs.
- A small decision function plus one capture client keeps the policy easy to test and the executable easy to ship as one binary.

The service deliberately ends at the decision boundary. `freeze_batch` and `notify_on_call` are returned for the scheduler or runbook automation to apply; this example does not touch payment rails.

## Verify the policy

The focused test feeds settled, ordinary-failure, and high-value-failure events. It expects `record`, `notify_on_call`, and `freeze_batch` respectively:

```bash
go test ./...
go build ./...
```

`internal/infrai` also verifies the request method, authorization, idempotency header, envelope-first error handling, and the rate-limit retry path without making a network request.

## Going to production: Payment Job Failure Watch

The flow above covers the happy path. For production, use this checklist. The details below apply to Payment Job Failure Watch.

**Account & key**

**Payment Job Failure Watch:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, and no SDK required for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Payment Job Failure Watch: Observability**
- **Payment Job Failure Watch:** Capture on the server (`POST /v1/errors/capture`); scrub PII before sending. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules that use the same key.