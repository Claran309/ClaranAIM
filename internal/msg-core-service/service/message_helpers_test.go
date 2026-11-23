package service

import (
	"ClaranAIM/internal/msg-core-service/model"
	"reflect"
	"testing"
	"time"
)

func TestDedupeUserIDs(t *testing.T) {
	got := dedupeUserIDs([]int64{0, -1, 3, 1, 3, 2, 1})
	want := []int64{3, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupeUserIDs() = %v, want %v", got, want)
	}
	if empty := dedupeUserIDs(nil); len(empty) != 0 {
		t.Fatalf("dedupeUserIDs(nil) = %v, want empty", empty)
	}
}

func TestMarshalMentionUserIDs(t *testing.T) {
	if got, err := marshalMentionUserIDs(nil); err != nil || got != "" {
		t.Fatalf("marshalMentionUserIDs(nil) = (%q, %v), want (\"\", nil)", got, err)
	}
	got, err := marshalMentionUserIDs([]int64{1, 2})
	if err != nil {
		t.Fatalf("marshalMentionUserIDs() error = %v", err)
	}
	if got != "[1,2]" {
		t.Fatalf("marshalMentionUserIDs() = %q, want [1,2]", got)
	}
}

func TestHydrateMessageRuntimeFields(t *testing.T) {
	msg := &model.Message{MentionsJSON: "[3,3,0,1]"}
	hydrateMessageRuntimeFields(msg)
	if !reflect.DeepEqual(msg.MentionUserIDs, []int64{3, 1}) {
		t.Fatalf("MentionUserIDs = %v, want [3 1]", msg.MentionUserIDs)
	}

	broken := &model.Message{MentionsJSON: "not-json"}
	hydrateMessageRuntimeFields(broken)
	if broken.MentionUserIDs != nil {
		t.Fatalf("broken JSON should not populate mentions, got %v", broken.MentionUserIDs)
	}
	hydrateMessageRuntimeFields(nil)
}

func TestFormatOptionalTime(t *testing.T) {
	if got := formatOptionalTime(nil); got != "" {
		t.Fatalf("formatOptionalTime(nil) = %q, want empty", got)
	}
	zero := time.Time{}
	if got := formatOptionalTime(&zero); got != "" {
		t.Fatalf("formatOptionalTime(zero) = %q, want empty", got)
	}
	value := time.Date(2026, 11, 23, 17, 0, 0, 0, time.Local)
	if got := formatOptionalTime(&value); got != "2026-11-23 17:00:00" {
		t.Fatalf("formatOptionalTime() = %q", got)
	}
}
