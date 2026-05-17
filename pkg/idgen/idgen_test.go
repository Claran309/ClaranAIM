package idgen

import (
	"sync"
	"testing"
)

func TestSnowflakeGeneratesUniquePositiveIDs(t *testing.T) {
	g, err := NewSnowflake(7)
	if err != nil {
		t.Fatal(err)
	}

	const total = 5000
	ids := make(map[int64]struct{}, total)
	for i := 0; i < total; i++ {
		id, err := g.NextID()
		if err != nil {
			t.Fatalf("NextID failed: %v", err)
		}
		if id <= 0 {
			t.Fatalf("expected positive id, got %d", id)
		}
		if _, ok := ids[id]; ok {
			t.Fatalf("duplicate id: %d", id)
		}
		ids[id] = struct{}{}
	}
}

func TestSnowflakeIsConcurrentSafe(t *testing.T) {
	g, err := NewSnowflake(3)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 16
	const perGoroutine = 500
	out := make(chan int64, goroutines*perGoroutine)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				id, err := g.NextID()
				if err != nil {
					t.Errorf("NextID failed: %v", err)
					return
				}
				out <- id
			}
		}()
	}
	wg.Wait()
	close(out)

	seen := map[int64]struct{}{}
	for id := range out {
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id: %d", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != goroutines*perGoroutine {
		t.Fatalf("expected %d ids, got %d", goroutines*perGoroutine, len(seen))
	}
}

func TestUID10Range(t *testing.T) {
	for i := 0; i < 1000; i++ {
		uid, err := NewUID10()
		if err != nil {
			t.Fatal(err)
		}
		if uid < 1000000000 || uid > 9999999999 {
			t.Fatalf("uid must be 10 digits, got %d", uid)
		}
	}
}
