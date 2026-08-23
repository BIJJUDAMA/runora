package model

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var shardRegex = regexp.MustCompile(`(?i)^(.+?)[._-](\d{1,5})-of-(\d{1,5})\.gguf$`)

type shardFileInfo struct {
	path       string
	info       os.FileInfo
	shardIndex int
	total      int
	basePrefix string
}

// DiscoverModels recursively scans the given root directory (or directories) for GGUF/ONNX files
// and parses their metadata, automatically grouping multi-file GGUF shards into single logical models
// and utilizing a metadata cache.
func DiscoverModels(roots ...string) ([]*GGUFMetadata, error) {
	if len(roots) == 0 {
		return nil, nil
	}

	cachePath := filepath.Join("cache", "metadata_cache.json")
	cache, _ := LoadCache(cachePath)
	if cache == nil {
		cache = NewMetadataCache()
	}

	cacheUpdated := false
	seenPaths := make(map[string]bool)

	var standaloneFiles []string
	var standaloneInfos []os.FileInfo
	shardGroups := make(map[string][]*shardFileInfo)

	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" {
			continue
		}
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			cleanPath := filepath.Clean(path)
			if seenPaths[cleanPath] {
				return nil
			}

			if !d.IsDir() {
				extLower := strings.ToLower(filepath.Ext(d.Name()))
				if extLower == ".gguf" || extLower == ".onnx" {
					info, statErr := d.Info()
					if statErr != nil {
						return nil
					}
					seenPaths[cleanPath] = true

					fileName := d.Name()
					if extLower == ".gguf" {
						if matches := shardRegex.FindStringSubmatch(fileName); len(matches) == 4 {
							basePrefix := matches[1]
							shardIdx, _ := strconv.Atoi(matches[2])
							totalShards, _ := strconv.Atoi(matches[3])
							dirKey := filepath.Dir(cleanPath)
							groupKey := dirKey + "||" + strings.ToLower(basePrefix)

							shardGroups[groupKey] = append(shardGroups[groupKey], &shardFileInfo{
								path:       cleanPath,
								info:       info,
								shardIndex: shardIdx,
								total:      totalShards,
								basePrefix: basePrefix,
							})
							return nil
						}
					}

					standaloneFiles = append(standaloneFiles, cleanPath)
					standaloneInfos = append(standaloneInfos, info)
				}
			}
			return nil
		})
	}

	var models []*GGUFMetadata

	// 1. Process Standalone Models
	for i, path := range standaloneFiles {
		info := standaloneInfos[i]
		extLower := strings.ToLower(filepath.Ext(path))
		cacheKey := filepath.ToSlash(path)

		var meta *GGUFMetadata
		entry, exists := cache.Entries[cacheKey]
		if exists && entry.ModTime == info.ModTime().Unix() && entry.Size == info.Size() {
			meta = entry.Metadata
			meta.FilePath = path
			if meta.ID == "" {
				meta.ID = filepath.Base(path)
			}
			if meta.Runtime == "" {
				if extLower == ".gguf" {
					meta.Runtime = "llama.cpp"
				} else if extLower == ".onnx" {
					meta.Runtime = "ONNX Runtime"
				}
			}
			if meta.Task == "" {
				if meta.EmbeddingLen > 0 {
					meta.Task = "EMBEDDING"
				} else {
					meta.Task = "TEXT_GENERATION"
				}
			}
			meta.ShardCount = 1
			meta.ShardFiles = []string{path}
		} else {
			if extLower == ".gguf" {
				var parseErr error
				meta, parseErr = ParseGGUF(path)
				if parseErr == nil {
					meta.ShardCount = 1
					meta.ShardFiles = []string{path}
					cache.Entries[cacheKey] = &GGUFCacheEntry{
						Metadata: meta,
						ModTime:  info.ModTime().Unix(),
						Size:     info.Size(),
					}
					cacheUpdated = true
				}
			} else if extLower == ".onnx" {
				meta = &GGUFMetadata{
					ID:           filepath.Base(path),
					Name:         filepath.Base(path),
					FilePath:     path,
					FileSize:     info.Size(),
					Runtime:      "ONNX Runtime",
					Task:         "TEXT_GENERATION",
					Architecture: "ONNX",
					Quantization: "Float32",
					ShardCount:   1,
					ShardFiles:   []string{path},
				}
				cache.Entries[cacheKey] = &GGUFCacheEntry{
					Metadata: meta,
					ModTime:  info.ModTime().Unix(),
					Size:     info.Size(),
				}
				cacheUpdated = true
			}
		}

		if meta != nil {
			applyTaskHeuristics(meta, path)
			models = append(models, meta)
		}
	}

	// 2. Process Multi-File GGUF Shard Groups
	for _, shards := range shardGroups {
		if len(shards) == 0 {
			continue
		}

		// Sort shards by index ascending
		sort.Slice(shards, func(i, j int) bool {
			return shards[i].shardIndex < shards[j].shardIndex
		})

		var totalAggregateSize int64
		var shardFilePaths []string
		expectedTotal := shards[0].total
		if len(shards) > expectedTotal {
			expectedTotal = len(shards)
		}

		for _, s := range shards {
			totalAggregateSize += s.info.Size()
			shardFilePaths = append(shardFilePaths, s.path)
		}

		// Primary shard to parse is shard 1 (or lowest available shard index)
		primaryShard := shards[0]
		primaryPath := primaryShard.path
		primaryInfo := primaryShard.info
		cacheKey := filepath.ToSlash(primaryPath)

		var meta *GGUFMetadata
		entry, exists := cache.Entries[cacheKey]
		if exists && entry.ModTime == primaryInfo.ModTime().Unix() && entry.Size == primaryInfo.Size() {
			meta = entry.Metadata
			meta.FilePath = primaryPath
			if meta.ID == "" {
				meta.ID = primaryShard.basePrefix + ".gguf"
			}
			if meta.Runtime == "" {
				meta.Runtime = "llama.cpp"
			}
			if meta.Task == "" {
				if meta.EmbeddingLen > 0 {
					meta.Task = "EMBEDDING"
				} else {
					meta.Task = "TEXT_GENERATION"
				}
			}
		} else {
			var parseErr error
			meta, parseErr = ParseGGUF(primaryPath)
			if parseErr == nil {
				cache.Entries[cacheKey] = &GGUFCacheEntry{
					Metadata: meta,
					ModTime:  primaryInfo.ModTime().Unix(),
					Size:     primaryInfo.Size(),
				}
				cacheUpdated = true
			}
		}

		if meta != nil {
			// Update meta with aggregated shard details
			meta.FileSize = totalAggregateSize
			meta.ShardCount = expectedTotal
			meta.ShardFiles = shardFilePaths
			if meta.Name == "" || meta.Name == filepath.Base(primaryPath) {
				meta.Name = primaryShard.basePrefix
			}
			applyTaskHeuristics(meta, primaryPath)
			models = append(models, meta)
		}
	}

	if cacheUpdated {
		_ = cache.Save(cachePath)
	}

	return models, nil
}

func applyTaskHeuristics(meta *GGUFMetadata, path string) {
	if meta == nil {
		return
	}
	dirLower := strings.ToLower(filepath.ToSlash(filepath.Dir(path)))
	if strings.Contains(dirLower, "/embedding") {
		meta.Task = "EMBEDDING"
	} else if strings.Contains(dirLower, "/speech") {
		meta.Task = "SPEECH_TO_TEXT"
	} else if strings.Contains(dirLower, "/vision") {
		meta.Task = "VISION"
	} else if strings.Contains(dirLower, "/diffusion") {
		meta.Task = "IMAGE_GENERATION"
	} else if strings.Contains(dirLower, "/reranker") {
		meta.Task = "RERANKING"
	}
}
