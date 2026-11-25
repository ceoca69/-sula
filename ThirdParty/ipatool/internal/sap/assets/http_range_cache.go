package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	maxPersistentRangeCacheSize  = int64(256 << 20)
	maxPersistentRangeCacheEntry = int64(8 << 20)
)

type cachedRemoteRange struct {
	offset  int64
	size    int64
	path    string
	digest  string
	modTime int64
}

type remoteRangeCache struct {
	mu     sync.RWMutex
	dir    string
	ranges []cachedRemoteRange
	size   int64
}

func newRemoteRangeCache(imageURL string) (*remoteRangeCache, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("locate remote image range cache: %w", err)
	}
	digest := sha256.Sum256([]byte(imageURL))
	directory := filepath.Join(root, "ipatool", "sap", "remote-ranges", hex.EncodeToString(digest[:8]))

	return openRemoteRangeCache(directory)
}

func openRemoteRangeCache(directory string) (*remoteRangeCache, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create remote image range cache: %w", err)
	}

	cache := &remoteRangeCache{dir: directory}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read remote image range cache: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		offset, size, digest, ok := parseCachedRangeName(entry.Name())
		if !ok || size <= 0 || size > maxPersistentRangeCacheEntry {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() != size {
			continue
		}
		cache.ranges = append(cache.ranges, cachedRemoteRange{
			offset: offset, size: size, path: filepath.Join(directory, entry.Name()), digest: digest, modTime: info.ModTime().UnixNano(),
		})
		cache.size += size
	}

	cache.pruneLocked("")
	if len(cache.ranges) != 0 {
		fmt.Printf("[ipatool/assets] loaded %d cached IPSW ranges (%d MiB)\n", len(cache.ranges), cache.size>>20)
	}
	return cache, nil
}

func parseCachedRangeName(name string) (int64, int64, string, bool) {
	if !strings.HasPrefix(name, "range-") || !strings.HasSuffix(name, ".bin") {
		return 0, 0, "", false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(name, "range-"), ".bin"), "-")
	if len(parts) != 3 || len(parts[2]) != 16 {
		return 0, 0, "", false
	}
	offset, offsetErr := strconv.ParseInt(parts[0], 16, 64)
	size, sizeErr := strconv.ParseInt(parts[1], 16, 64)
	_, digestErr := strconv.ParseUint(parts[2], 16, 64)
	return offset, size, parts[2], offsetErr == nil && sizeErr == nil && digestErr == nil && offset >= 0 && size > 0
}

func (c *remoteRangeCache) ReadAt(data []byte, offset int64) bool {
	if c == nil || len(data) == 0 {
		return false
	}
	end := offset + int64(len(data))
	c.mu.RLock()
	var match *cachedRemoteRange
	for index := range c.ranges {
		item := &c.ranges[index]
		if item.offset <= offset && item.offset+item.size >= end {
			if match == nil || item.size < match.size {
				copy := *item
				match = &copy
			}
		}
	}
	c.mu.RUnlock()
	if match == nil {
		return false
	}

	cached, err := os.ReadFile(match.path)
	if err != nil || int64(len(cached)) != match.size {
		c.remove(match.path)
		return false
	}
	digest := sha256.Sum256(cached)
	if hex.EncodeToString(digest[:8]) != match.digest {
		c.remove(match.path)
		return false
	}
	start := offset - match.offset
	copy(data, cached[start:start+int64(len(data))])
	return true
}

func (c *remoteRangeCache) Store(data []byte, offset int64) error {
	if c == nil || len(data) == 0 || int64(len(data)) > maxPersistentRangeCacheEntry || offset < 0 {
		return nil
	}
	digest := sha256.Sum256(data)
	digestText := hex.EncodeToString(digest[:8])
	name := fmt.Sprintf("range-%016x-%016x-%s.bin", offset, len(data), digestText)
	path := filepath.Join(c.dir, name)
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	temporary, err := os.CreateTemp(c.dir, ".range-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil
		}
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	c.mu.Lock()
	for _, item := range c.ranges {
		if item.path == path {
			c.mu.Unlock()
			return nil
		}
	}
	c.ranges = append(c.ranges, cachedRemoteRange{offset: offset, size: int64(len(data)), path: path, digest: digestText, modTime: info.ModTime().UnixNano()})
	c.size += int64(len(data))
	c.pruneLocked(path)
	c.mu.Unlock()
	return nil
}

func (c *remoteRangeCache) remove(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index := 0; index < len(c.ranges); index++ {
		if c.ranges[index].path != path {
			continue
		}
		c.size -= c.ranges[index].size
		c.ranges = append(c.ranges[:index], c.ranges[index+1:]...)
		_ = os.Remove(path)
		return
	}
}

func (c *remoteRangeCache) pruneLocked(keepPath string) {
	if c.size <= maxPersistentRangeCacheSize {
		return
	}
	oldest := append([]cachedRemoteRange(nil), c.ranges...)
	sort.Slice(oldest, func(i, j int) bool { return oldest[i].modTime < oldest[j].modTime })
	for _, item := range oldest {
		if c.size <= maxPersistentRangeCacheSize || item.path == keepPath {
			continue
		}
		if err := os.Remove(item.path); err == nil || errors.Is(err, os.ErrNotExist) {
			c.size -= item.size
			for index := range c.ranges {
				if c.ranges[index].path == item.path {
					c.ranges = append(c.ranges[:index], c.ranges[index+1:]...)
					break
				}
			}
		}
	}
}
