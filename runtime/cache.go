package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache global state
var (
	redisClient  *redis.Client
	localCache   = make(map[string]localCacheEntry)
	localCacheMu sync.RWMutex
)

type localCacheEntry struct {
	value      interface{}
	expiration time.Time
}

// Redis & In-Memory Cache
func InitCache(connStr string) {
	if strings.HasPrefix(connStr, "redis://") {
		opt, err := redis.ParseURL(connStr)
		if err != nil {
			panic(fmt.Sprintf("Invalid Redis URL: %s", err.Error()))
		}
		redisClient = redis.NewClient(opt)
		LogInfo("Connected to Redis cache: ", connStr)
	} else {
		LogInfo("Initialized in-memory cache fallback")
	}
}

func CacheSet(key string, value interface{}, durationStr string) {
	endSpan := TraceCache("SET", key)
	defer endSpan()

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		duration = 10 * time.Minute // default fallback
	}

	if redisClient != nil {
		b, _ := json.Marshal(value)
		err := redisClient.Set(dbCtx, key, string(b), duration).Err()
		if err != nil {
			panic(fmt.Sprintf("Redis SET error: %s", err.Error()))
		}
	} else {
		localCacheMu.Lock()
		localCache[key] = localCacheEntry{
			value:      value,
			expiration: time.Now().Add(duration),
		}
		localCacheMu.Unlock()
	}
}

func CacheGet(key string) interface{} {
	endSpan := TraceCache("GET", key)
	defer endSpan()

	if redisClient != nil {
		val, err := redisClient.Get(dbCtx, key).Result()
		if err == redis.Nil {
			return nil
		} else if err != nil {
			panic(fmt.Sprintf("Redis GET error: %s", err.Error()))
		}
		var parsed interface{}
		if err := json.Unmarshal([]byte(val), &parsed); err == nil {
			return parsed
		}
		return val
	} else {
		localCacheMu.RLock()
		entry, exists := localCache[key]
		localCacheMu.RUnlock()

		if !exists {
			return nil
		}
		if time.Now().After(entry.expiration) {
			localCacheMu.Lock()
			delete(localCache, key)
			localCacheMu.Unlock()
			return nil
		}
		return entry.value
	}
}
