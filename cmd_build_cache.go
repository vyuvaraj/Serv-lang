package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// BuildCache tracks source file hashes and generated code hashes
// to skip codegen when nothing changed.
type BuildCache struct {
	Version     string                 `json:"version"`
	Entries     map[string]CacheEntry  `json:"entries"`
	GeneratedHash string              `json:"generated_hash"`
	LastBuild   time.Time             `json:"last_build"`
}

type CacheEntry struct {
	SourceHash string    `json:"source_hash"`
	ModTime    time.Time `json:"mod_time"`
}

const buildCacheVersion = "1"
const buildCacheFile = ".serv-build-cache.json"

// loadBuildCache reads the cache from the build directory.
func loadBuildCache(buildDir string) *BuildCache {
	data, err := os.ReadFile(filepath.Join(buildDir, buildCacheFile))
	if err != nil {
		return &BuildCache{Version: buildCacheVersion, Entries: make(map[string]CacheEntry)}
	}
	var cache BuildCache
	if err := json.Unmarshal(data, &cache); err != nil || cache.Version != buildCacheVersion {
		return &BuildCache{Version: buildCacheVersion, Entries: make(map[string]CacheEntry)}
	}
	return &cache
}

// saveBuildCache writes the cache to the build directory.
func saveBuildCache(buildDir string, cache *BuildCache) {
	cache.LastBuild = time.Now()
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(buildDir, buildCacheFile), data, 0644)
}

// hashFile returns the SHA256 hex hash of a file's content.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// hashString returns the SHA256 hex hash of a string.
func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// isSourceUnchanged checks if all source files in the project
// have the same hash as the last successful build.
func isSourceUnchanged(cache *BuildCache, sourceFiles []string) bool {
	if len(cache.Entries) == 0 {
		return false
	}
	if len(sourceFiles) != len(cache.Entries) {
		return false // File count changed (new file added or file removed)
	}
	for _, f := range sourceFiles {
		hash, err := hashFile(f)
		if err != nil {
			return false
		}
		entry, exists := cache.Entries[f]
		if !exists || entry.SourceHash != hash {
			return false
		}
	}
	return true
}

// updateCacheEntries records the current hashes of all source files.
func updateCacheEntries(cache *BuildCache, sourceFiles []string) {
	cache.Entries = make(map[string]CacheEntry)
	for _, f := range sourceFiles {
		hash, err := hashFile(f)
		if err != nil {
			continue
		}
		info, _ := os.Stat(f)
		modTime := time.Time{}
		if info != nil {
			modTime = info.ModTime()
		}
		cache.Entries[f] = CacheEntry{
			SourceHash: hash,
			ModTime:    modTime,
		}
	}
}

// isGeneratedCodeUnchanged checks if the generated Go code matches
// what was last written. If true, we can skip `go build` entirely
// (the Go binary is already up-to-date).
func isGeneratedCodeUnchanged(cache *BuildCache, generatedCode string) bool {
	return cache.GeneratedHash != "" && cache.GeneratedHash == hashString(generatedCode)
}

// collectSourceFiles finds all .srv files in a project directory
// (or returns a single file for single-file builds).
func collectSourceFiles(srvFile string) ([]string, error) {
	fi, err := os.Stat(srvFile)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		abs, _ := filepath.Abs(srvFile)
		return []string{abs}, nil
	}
	var files []string
	err = filepath.Walk(srvFile, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".srv" {
			abs, _ := filepath.Abs(path)
			files = append(files, abs)
		}
		return nil
	})
	return files, err
}
