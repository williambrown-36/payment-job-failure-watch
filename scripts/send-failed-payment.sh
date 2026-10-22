#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
    "job_name": "settlement-close",
    "run_id": "run-2026-10-01-001",
    "payment_id": "pay-8831",
    "amount_minor": 725000,
    "currency": "USD",
    "state": "failed",
    "failure_reason": "ledger balance check rejected the settlement",
    "occurred_at": "2026-10-01T03:00:00Z"
  }' \
  http://localhost:8080/payment-events
