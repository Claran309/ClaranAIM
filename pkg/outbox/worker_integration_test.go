//go:build integration

package outbox

import (
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const outboxIntegrationDSNEnv = "CLARAN_TEST_MYSQL_DSN"

func setupOutboxDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(outboxIntegrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未设置，跳过 outbox MySQL 集成测试", outboxIntegrationDSNEnv)
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := db.Exec("DELETE FROM event_outbox").Error; err != nil {
		t.Fatalf("clean event_outbox error = %v", err)
	}
	return db
}

func seedOutboxEvent(t *testing.T, db *gorm.DB, aggregateID int64) Event {
	t.Helper()
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "conversation-1", map[string]string{"content": "hello"})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	event := Event{
		AggregateType: "message",
		AggregateID:   aggregateID,
		EventType:     envelope.Type,
		EventKey:      envelope.Key,
		Payload:       mustEnvelopeJSON(t, envelope),
		Status:        StatusPending,
		NextRetryAt:   time.Now().Add(-time.Second),
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create outbox event error = %v", err)
	}
	return event
}

func mustEnvelopeJSON(t *testing.T, envelope events.Envelope) string {
	t.Helper()
	data, err := envelope.Bytes()
	if err != nil {
		t.Fatalf("envelope.Bytes() error = %v", err)
	}
	return string(data)
}

func TestIntegrationWorkerPublishesAndMarksPublished(t *testing.T) {
	db := setupOutboxDB(t)
	store := NewGormStore(db)
	event := seedOutboxEvent(t, db, 1001)
	publisher := &recordingPublisher{}
	worker := NewWorker(store, publisher)
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published = %d, want 1", len(publisher.events))
	}
	var stored Event
	if err := db.First(&stored, event.ID).Error; err != nil {
		t.Fatalf("reload event error = %v", err)
	}
	if stored.Status != StatusPublished || stored.PublishedAt == nil {
		t.Fatalf("event = %#v, want published", stored)
	}
	backlog, err := store.CountBacklog(context.Background())
	if err != nil {
		t.Fatalf("CountBacklog() error = %v", err)
	}
	if backlog != 0 {
		t.Fatalf("backlog = %d, want 0", backlog)
	}
}

func TestIntegrationWorkerMarksRetryOnPublishFailure(t *testing.T) {
	db := setupOutboxDB(t)
	store := NewGormStore(db)
	event := seedOutboxEvent(t, db, 1002)
	worker := NewWorker(store, &recordingPublisher{err: errors.New("kafka unavailable")})
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	var stored Event
	if err := db.First(&stored, event.ID).Error; err != nil {
		t.Fatalf("reload event error = %v", err)
	}
	if stored.Status != StatusRetrying || stored.RetryCount != 1 || stored.LastError == "" {
		t.Fatalf("event = %#v, want retrying with error", stored)
	}
	if stored.LockedUntil != nil {
		t.Fatal("locked_until should be released after retry scheduling")
	}
}

func TestIntegrationWorkerMarksDeadAfterMaxRetries(t *testing.T) {
	db := setupOutboxDB(t)
	store := NewGormStore(db)
	event := seedOutboxEvent(t, db, 1003)
	if err := db.Model(&Event{}).Where("id = ?", event.ID).Update("retry_count", 2).Error; err != nil {
		t.Fatalf("seed retry_count error = %v", err)
	}
	worker := NewWorker(store, &recordingPublisher{err: errors.New("permanent")})
	worker.SetMaxRetries(3)
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	var stored Event
	if err := db.First(&stored, event.ID).Error; err != nil {
		t.Fatalf("reload event error = %v", err)
	}
	if stored.Status != StatusDead {
		t.Fatalf("status = %q, want dead", stored.Status)
	}
}

func TestIntegrationWorkerSkipsEventLockedByAnotherWorker(t *testing.T) {
	db := setupOutboxDB(t)
	store := NewGormStore(db)
	event := seedOutboxEvent(t, db, 1004)
	lockedUntil := time.Now().Add(time.Minute)
	if err := db.Model(&Event{}).Where("id = ?", event.ID).Update("locked_until", lockedUntil).Error; err != nil {
		t.Fatalf("lock event error = %v", err)
	}
	publisher := &recordingPublisher{}
	worker := NewWorker(store, publisher)
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published %d locked events, want 0", len(publisher.events))
	}
}

type recordingPublisher struct {
	events []events.Envelope
	err    error
}

func (p *recordingPublisher) Publish(_ context.Context, envelope events.Envelope) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, envelope)
	return nil
}
