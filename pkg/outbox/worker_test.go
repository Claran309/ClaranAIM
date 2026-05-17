package outbox

import (
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	records       []Event
	publishedIDs  []int64
	retriedIDs    []int64
	retryErrs     []string
	fetchCalled   bool
	publishCalled bool
}

func (s *fakeStore) FetchDue(ctx context.Context, limit int, lockFor time.Duration) ([]Event, error) {
	s.fetchCalled = true
	return s.records, nil
}

func (s *fakeStore) MarkPublished(ctx context.Context, id int64) error {
	s.publishedIDs = append(s.publishedIDs, id)
	return nil
}

func (s *fakeStore) MarkRetry(ctx context.Context, id int64, publishErr error) error {
	s.retriedIDs = append(s.retriedIDs, id)
	s.retryErrs = append(s.retryErrs, publishErr.Error())
	return nil
}

type fakePublisher struct {
	err       error
	published []events.Envelope
}

func (p *fakePublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, envelope)
	return nil
}

func (p *fakePublisher) Close() error {
	return nil
}

func TestWorkerMarksEventPublishedAfterKafkaPublishSucceeds(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "1", map[string]int64{"msg_id": 42})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	record, err := NewEvent("message", 42, envelope)
	if err != nil {
		t.Fatalf("NewEvent returned error: %v", err)
	}
	store := &fakeStore{records: []Event{record}}
	publisher := &fakePublisher{}
	worker := NewWorker(store, publisher)

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce returned error: %v", err)
	}

	if len(publisher.published) != 1 || publisher.published[0].EventID != envelope.EventID {
		t.Fatalf("published envelopes = %#v, want event_id %s", publisher.published, envelope.EventID)
	}
	if len(store.publishedIDs) != 1 || store.publishedIDs[0] != record.ID {
		t.Fatalf("published ids = %#v, want [%d]", store.publishedIDs, record.ID)
	}
	if len(store.retriedIDs) != 0 {
		t.Fatalf("retried ids = %#v, want none", store.retriedIDs)
	}
}

func TestWorkerRetriesEventWhenKafkaPublishFails(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeGroupCreated, "10", map[string]int64{"group_id": 10})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	record, err := NewEvent("group", 10, envelope)
	if err != nil {
		t.Fatalf("NewEvent returned error: %v", err)
	}
	store := &fakeStore{records: []Event{record}}
	publisher := &fakePublisher{err: errors.New("kafka unavailable")}
	worker := NewWorker(store, publisher)

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce returned error: %v", err)
	}

	if len(store.publishedIDs) != 0 {
		t.Fatalf("published ids = %#v, want none", store.publishedIDs)
	}
	if len(store.retriedIDs) != 1 || store.retriedIDs[0] != record.ID {
		t.Fatalf("retried ids = %#v, want [%d]", store.retriedIDs, record.ID)
	}
	if len(store.retryErrs) != 1 || store.retryErrs[0] != "kafka unavailable" {
		t.Fatalf("retry errors = %#v, want kafka unavailable", store.retryErrs)
	}
}
