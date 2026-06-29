package cmd

import (
	"testing"

	"github.com/alexander-posztos/ccdash/internal/config"
)

// TestAcquireSingletonContention verifies the lock semantics behind --singleton:
// the first holder wins, a concurrent attempt is told it does NOT hold the lock
// (so its caller exits 0 and the WM focuses the existing window), and the lock is
// reusable once released.
func TestAcquireSingletonContention(t *testing.T) {
	cfg := config.Config{CacheDir: t.TempDir()}

	lock1, held1, err := acquireSingleton(cfg)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if !held1 {
		t.Fatal("first acquire should own the lock")
	}

	if _, held2, err := acquireSingleton(cfg); err != nil {
		t.Fatalf("second acquire errored: %v", err)
	} else if held2 {
		t.Fatal("second acquire must not own the lock while the first holds it")
	}

	if err := lock1.Unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	lock3, held3, err := acquireSingleton(cfg)
	if err != nil || !held3 {
		t.Fatalf("after release the lock should be re-acquirable: held=%v err=%v", held3, err)
	}
	_ = lock3.Unlock()
}
