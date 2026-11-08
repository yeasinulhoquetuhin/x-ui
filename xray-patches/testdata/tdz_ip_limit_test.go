package dispatcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	corestats "github.com/xtls/xray-core/app/stats"
)

func TestTDZLimitSharedSlotsReleaseAndCanonicalIPv4(t *testing.T) {
	limiter := tdzIPLimiter{limits: map[string]int{"client": 1}}
	online := corestats.NewOnlineMap()
	_, release1, err := limiter.acquire(context.Background(), "client", "192.0.2.1", "", online)
	if err != nil {
		t.Fatal(err)
	}
	defer release1()
	_, release2, err := limiter.acquire(context.Background(), "client", "::ffff:192.0.2.1", "", online)
	if err != nil {
		t.Fatal(err)
	}
	defer release2()
	if online.Count() != 1 {
		t.Fatalf("same IP must consume one slot: %d", online.Count())
	}
	if _, _, err := limiter.acquire(context.Background(), "client", "2001:db8::2", "", online); err == nil {
		t.Fatal("second IP admitted")
	}
	release1()
	if _, _, err := limiter.acquire(context.Background(), "client", "192.0.2.2", "", online); err == nil {
		t.Fatal("released a slot with another live stream")
	}
	release2()
	if online.Count() != 0 {
		t.Fatal("offline IP retained")
	}
	_, release3, err := limiter.acquire(context.Background(), "client", "[2001:db8::2]", "", online)
	if err != nil {
		t.Fatal(err)
	}
	release3()
	if online.Count() != 0 {
		t.Fatal("IPv6 slot retained after close")
	}
}

func TestTDZLimitConcurrentAdmission(t *testing.T) {
	limiter := tdzIPLimiter{limits: map[string]int{"client": 1}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var releases []func()
	for i := 1; i <= 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, release, err := limiter.acquire(context.Background(), "client", fmt.Sprintf("192.0.2.%d", i), "", nil)
			if err == nil {
				mu.Lock()
				releases = append(releases, release)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	if len(releases) != 1 {
		t.Fatalf("expected 1 IP, admitted %d", len(releases))
	}
	_, other, err := limiter.acquire(context.Background(), "other-client", "203.0.113.1", "", nil)
	if err != nil {
		t.Fatal("one user's limit affected another", err)
	}
	other()
}

func TestTDZLimitReloadReducesExistingConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limits.json")
	if err := os.WriteFile(path, []byte(`{"client":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	var limiter tdzIPLimiter
	ctx1, release1, err := limiter.acquire(context.Background(), "client", "192.0.2.1", path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release1()
	ctx2, release2, err := limiter.acquire(context.Background(), "client", "192.0.2.2", path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release2()
	newPath := path + ".new"
	if err := os.WriteFile(newPath, []byte(`{"client":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(newPath, path); err != nil {
		t.Fatal(err)
	}
	limiter.reload(path)
	select {
	case <-ctx2.Done():
	case <-time.After(time.Second):
		t.Fatal("excess stream not cancelled")
	}
	if ctx1.Err() != nil {
		t.Fatal("original connection cancelled")
	}
	if err := os.WriteFile(newPath, []byte(`{"client":0}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(newPath, path); err != nil {
		t.Fatal(err)
	}
	_, release3, err := limiter.acquire(context.Background(), "client", "192.0.2.3", path, nil)
	if err != nil {
		t.Fatal("removing limit did not take effect", err)
	}
	release3()
}

func TestTDZLimitMissingOrInvalidConfigDoesNotSilentlyDisable(t *testing.T) {
	var limiter tdzIPLimiter
	path := filepath.Join(t.TempDir(), "missing.json")
	if _, _, err := limiter.acquire(context.Background(), "client", "192.0.2.1", path, nil); err == nil {
		t.Fatal("missing limits silently accepted")
	}
	if err := os.WriteFile(path, []byte(`invalid`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := limiter.acquire(context.Background(), "client", "192.0.2.1", path, nil); err == nil {
		t.Fatal("invalid limits silently accepted")
	}
}
