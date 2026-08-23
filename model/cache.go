package model

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type GGUFCacheEntry struct {
	Metadata *GGUFMetadata `json:"metadata"`
	ModTime  int64         `json:"mod_time"` // Unix timestamp
	Size     int64         `json:"size"`
}

type MetadataCache struct {
	Entries map[string]*GGUFCacheEntry `json:"entries"`
}

func NewMetadataCache() *MetadataCache {
	return &MetadataCache{
		Entries: make(map[string]*GGUFCacheEntry),
	}
}

func LoadCache(cachePath string) (*MetadataCache, error) {
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return NewMetadataCache(), nil
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	cache := NewMetadataCache()
	if err := json.Unmarshal(data, cache); err != nil {
		return NewMetadataCache(), nil // If corrupt, return empty cache
	}
	return cache, nil
}

func (c *MetadataCache) Save(cachePath string) error {
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return ReplaceFile(tmpPath, cachePath)
}
