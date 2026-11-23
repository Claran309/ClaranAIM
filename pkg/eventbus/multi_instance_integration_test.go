//go:build integration

package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const eventbusIntegrationDSNEnv = "CLARAN_TEST_MYSQL_DSN"

func setupReliabilityDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(eventbusIntegrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未设置，跳过 eventbus MySQL 集成测试", eventbusIntegrationDSNEnv)
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := AutoMigrateReliabilityStore(db); err != nil {
		t.Fatalf("AutoMigrateReliabilityStore() error = %v", err)
	}
	if err := db.Exec("DELETE FROM processed_events").Error; err != nil {
		t.Fatalf("clean processed_events error = %v", err)
	}
	return db
}

// TestIntegrationTwoGormInstancesClaimOnce 验证两个真实 consumer 实例在 MySQL 上只执行一次。
func TestIntegrationTwoGormInstancesClaimOnce(t *testing.T) {
	db := setupReliabilityDB(t)
	storeA := NewGormReliabilityStore(db)
	storeB := NewGormReliabilityStore(db)
	var executed int64
	handlerA := NewReliableHandler(storeA, "websocket-gateway", 3, func(context.Context, events.Envelope) error {
		atomic.AddInt64(&executed, 1)
		return nil
	})
	handlerB := NewReliableHandler(storeB, "websocket-gateway", 3, func(context.Context, events.Envelope) error {
		atomic.AddInt64(&executed, 1)
		return nil
	})
	envelope := testEnvelope(t, "event-mysql-concurrent")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		handler := handlerA
		if i%2 == 1 {
			handler = handlerB
		}
		go func(h Handler) {
			defer wg.Done()
			if err := h(context.Background(), envelope); err != nil {
				t.Errorf("handler error = %v", err)
			}
		}(handler)
	}
	wg.Wait()

	if got := atomic.LoadInt64(&executed); got != 1 {
		t.Fatalf("executed = %d, want exactly 1 across MySQL-backed instances", got)
	}
	key := ConsumerEventKey("websocket-gateway", envelope)
	if ok, err := storeA.IsProcessed(context.Background(), key); err != nil || !ok {
		t.Fatalf("IsProcessed() = (%v, %v), want (true, nil)", ok, err)
	}
}

// TestIntegrationFailedRecordCanBeReclaimed 验证失败记录可被另一实例重新抢占。
func TestIntegrationFailedRecordCanBeReclaimed(t *testing.T) {
	db := setupReliabilityDB(t)
	store := NewGormReliabilityStore(db)
	ctx := context.Background()
	envelope := testEnvelope(t, "event-mysql-retry")
	key := ConsumerEventKey("consumer", envelope)
	if ok, err := store.TryClaim(ctx, key, envelope); err != nil || !ok {
		t.Fatalf("first claim = (%v, %v)", ok, err)
	}
	if _, err := store.RecordFailure(ctx, key, envelope, assertError("boom")); err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}
	if ok, err := store.TryClaim(ctx, key, envelope); err != nil || !ok {
		t.Fatalf("reclaim after failure = (%v, %v), want true", ok, err)
	}
	if err := store.MarkProcessed(ctx, key, envelope); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}
	if ok, err := store.TryClaim(ctx, key, envelope); err != nil || ok {
		t.Fatalf("claim after processed = (%v, %v), want false", ok, err)
	}
}
