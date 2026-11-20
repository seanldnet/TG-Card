package database

import (
	"context"
	"testing"
	"time"
)

func TestSystemExceptionUpsertFilteringAndResolution(t *testing.T) {
	ctx := context.Background()
	db := reconciliationTestDB(t)
	sourceKey := "delivery-uncertain:test-order"
	if err := CreateSystemException(ctx, db, "sqlite", sourceKey, "delivery_uncertain", "error", "order", "test-order", "订单交付结果不确定", "first failure"); err != nil {
		t.Fatal(err)
	}
	if err := CreateSystemException(ctx, db, "sqlite", sourceKey, "delivery_uncertain", "critical", "order", "test-order", "订单交付结果不确定", "second failure"); err != nil {
		t.Fatal(err)
	}
	items, err := ListSystemExceptions(ctx, db, "sqlite", "open", "delivery_uncertain", time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano), time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].OccurrenceCount != 2 || items[0].Severity != "critical" || items[0].Detail != "second failure" {
		t.Fatalf("expected a single aggregated exception with two occurrences: %+v", items)
	}
	if items[0].LastSeenAt == "" {
		t.Fatal("last_seen_at must be populated")
	}
	if err := UpdateSystemException(ctx, db, "sqlite", items[0].ID, "admin-1", "resolved", "已核实订单并完成补发"); err != nil {
		t.Fatal(err)
	}
	count, err := CountOpenSystemExceptions(ctx, db)
	if err != nil || count != 0 {
		t.Fatalf("resolved exception should not be counted as open: count=%d err=%v", count, err)
	}
	if err := CreateSystemException(ctx, db, "sqlite", sourceKey, "delivery_uncertain", "error", "order", "test-order", "订单交付结果不确定", "delivery became uncertain again"); err != nil {
		t.Fatal(err)
	}
	items, err = ListSystemExceptions(ctx, db, "sqlite", "open", "delivery_uncertain", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].OccurrenceCount != 3 || items[0].Status != "open" || items[0].ResolutionNote != "" {
		t.Fatalf("repeated exception should reopen and increment occurrence count: %+v", items)
	}
}

func TestSystemExceptionDateFiltering(t *testing.T) {
	ctx := context.Background()
	db := reconciliationTestDB(t)
	if err := CreateSystemException(ctx, db, "sqlite", "cap-verification:today", "cap_verification_failed", "warning", "security", "admin-login", "Cap 验证失败", "token rejected"); err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339Nano)
	items, err := ListSystemExceptions(ctx, db, "sqlite", "", "", future, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("future start date should exclude current exception: %+v", items)
	}
}
