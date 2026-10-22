package monitor

import (
	"testing"
	"time"
)

func TestDecidePaymentAction(t *testing.T) {
	tests := []struct {
		name  string
		event PaymentEvent
		want  Decision
	}{
		{
			name:  "successful payment is recorded",
			event: PaymentEvent{State: "settled", AmountMinor: 900_000},
			want:  Decision{Action: ActionRecord, Severity: "info", Reason: "payment completed without an alert"},
		},
		{
			name:  "ordinary failure pages on-call",
			event: PaymentEvent{State: "failed", AmountMinor: 49_999},
			want:  Decision{Action: ActionNotifyOnCall, Severity: "error", Reason: "payment failed"},
		},
		{
			name:  "threshold failure freezes the batch",
			event: PaymentEvent{State: "failed", AmountMinor: 50_000},
			want:  Decision{Action: ActionFreezeBatch, Severity: "critical", Reason: "high-value payment failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.event, 50_000); got != tt.want {
				t.Fatalf("Decide() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestValidateRequiresFailureReason(t *testing.T) {
	event := PaymentEvent{
		JobName: "settlement-close", RunID: "run-1042", PaymentID: "pay-8831",
		AmountMinor: 12500, Currency: "USD", State: "failed", OccurredAt: time.Unix(1, 0),
	}
	if err := Validate(event); err == nil {
		t.Fatal("Validate() accepted a failed payment without failure_reason")
	}
}
