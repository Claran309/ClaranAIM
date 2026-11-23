//go:build integration

package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// TestIntegrationCrashedConsumerLeaseIsReclaimed 覆盖 A1 的“消费者崩溃重试”：
// 实例 A 抢占后崩溃（只留下 processing 记录、没有成功/失败），租约过期后实例 B 必须能接管。
func TestIntegrationCrashedConsumerLeaseIsReclaimed(t *testing.T) {
	db := setupReliabilityDB(t)
	storeA := NewGormReliabilityStore(db)
	storeB := NewGormReliabilityStore(db)
	ctx := context.Background()
	envelope := testEnvelope(t, "event-crash-recovery")
	key := ConsumerEventKey("websocket-gateway", envelope)

	if claimed, err := storeA.TryClaim(ctx, key, envelope); err != nil || !claimed {
		t.Fatalf("instance A claim = (%v, %v), want true", claimed, err)
	}
	if claimed, err := storeB.TryClaim(ctx, key, envelope); err != nil || claimed {
		t.Fatalf("instance B claim during active lease = (%v, %v), want false", claimed, err)
	}

	// 模拟实例 A 崩溃：记录停留在 processing，且 updated_at 早于租约窗口。
	stale := time.Now().Add(-staleProcessingWindow - time.Minute)
	if err := db.Model(&GormDeliveryRecord{}).Where("`key` = ?", key).
		Updates(map[string]interface{}{"state": DeliveryStateProcessing, "updated_at": stale}).Error; err != nil {
		t.Fatalf("simulate crash error = %v", err)
	}

	if claimed, err := storeB.TryClaim(ctx, key, envelope); err != nil || !claimed {
		t.Fatalf("instance B takeover after lease expiry = (%v, %v), want true", claimed, err)
	}
	if err := storeB.MarkProcessed(ctx, key, envelope); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}
	if ok, err := storeA.IsProcessed(ctx, key); err != nil || !ok {
		t.Fatalf("IsProcessed() = (%v, %v), want true", ok, err)
	}
}
