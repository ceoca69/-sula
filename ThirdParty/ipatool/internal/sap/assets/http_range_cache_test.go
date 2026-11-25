package assets

import (
	"bytes"
	"os"
	"testing"
)

func TestRemoteRangeCachePersistsAndReadsContainingRanges(t *testing.T) {
	directory := t.TempDir()
	cache, err := openRemoteRangeCache(directory)
	if err != nil {
		t.Fatalf("open range cache: %v", err)
	}

	original := []byte("persistent remote IPSW range")
	if err := cache.Store(original, 4096); err != nil {
		t.Fatalf("store range: %v", err)
	}

	reloaded, err := openRemoteRangeCache(directory)
	if err != nil {
		t.Fatalf("reload range cache: %v", err)
	}

	result := make([]byte, len(original)-10)
	if !reloaded.ReadAt(result, 4106) {
		t.Fatal("expected cached range to contain requested bytes")
	}
	if !bytes.Equal(result, original[10:]) {
		t.Fatalf("cached bytes = %q, want %q", result, original[10:])
	}
}

func TestRemoteRangeCacheIgnoresCorruptEntries(t *testing.T) {
	directory := t.TempDir()
	cache, err := openRemoteRangeCache(directory)
	if err != nil {
		t.Fatalf("open range cache: %v", err)
	}
	if err := cache.Store([]byte("valid"), 12); err != nil {
		t.Fatalf("store range: %v", err)
	}

	for _, item := range cache.ranges {
		if err := os.WriteFile(item.path, []byte("wrong"), 0o600); err != nil {
			t.Fatalf("corrupt cached range: %v", err)
		}
	}
	if reloaded, err := openRemoteRangeCache(directory); err != nil {
		t.Fatalf("reload range cache: %v", err)
	} else if reloaded.ReadAt(make([]byte, 5), 12) {
		t.Fatal("corrupt range must not be used")
	}
}
