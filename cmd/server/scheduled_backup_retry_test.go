package main

import (
	"testing"
	"time"
)

func TestScheduledBackupRetryDue(t *testing.T) {
	now := time.Date(2026, 11, 22, 4, 0, 0, 0, time.Local)
	tests := []struct {
		name string
		lastSuccess string
		lastAttempt string
		want bool
	}{
		{name: "already succeeded today", lastSuccess: now.Format("2006-01-02"), want: false},
		{name: "first attempt today", want: true},
		{name: "recent failure waits", lastAttempt: now.Add(-10 * time.Minute).UTC().Format(time.RFC3339Nano), want: false},
		{name: "failed attempt retries after interval", lastAttempt: now.Add(-31 * time.Minute).UTC().Format(time.RFC3339Nano), want: true},
		{name: "invalid attempt timestamp does not block forever", lastAttempt: "invalid", want: true},
		{name: "previous day success does not block today", lastSuccess: now.AddDate(0, 0, -1).Format("2006-01-02"), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scheduledBackupRetryDue(now, test.lastSuccess, test.lastAttempt); got != test.want {
				t.Fatalf("scheduledBackupRetryDue() = %v, want %v", got, test.want)
			}
		})
	}
}
