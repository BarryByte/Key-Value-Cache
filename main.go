package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"strings" // Needed for TrimSpace
	"sync"
	"unicode/utf8" // Needed for correct character count
)

// --- Constants --- (Assuming previous constants NumShards, MaxCapacityPerShard, etc. are here)
const (
	NumShards           = 64
	MaxCapacityPerShard = 4096
	TotalCapacity       = NumShards * MaxCapacityPerShard
	MaxKeyLength        = 256
	MaxValueLength      = 256
)

// --- Request & Response Models (Updated for new spec) ---

// PutRequest remains the same structure for decoding
type PutRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GenericErrorResponse structure for standard error replies
type GenericErrorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// PutSuccessResponse structure for PUT success replies
type PutSuccessResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// GetSuccessResponse structure for GET success replies
type GetSuccessResponse struct {
	Status string `json:"status"`
	Key    string `json:"key"`
	Value  string `json:"value"`
}


// --- LRU Cache Implementation ---

// entry represents a key-value pair in the LRU cache's linked list.
type entry struct {
	key   string
	value string
}

// LRUCache holds the data for a single cache shard with LRU eviction.
type LRUCache struct {
	mutex    sync.Mutex // Use Mutex as writes require exclusive access to list+map
	capacity int
	items    map[string]*list.Element // Map key to list element for O(1) access
	evictList *list.List              // Doubly linked list for O(1) add/remove/move
}

// NewLRUCache initializes a new LRU cache shard.
func NewLRUCache(capacity int) *LRUCache {
	if capacity <= 0 {
		// Default or minimum capacity if needed, but better to configure properly.
		capacity = MaxCapacityPerShard
	}
	return &LRUCache{
		capacity:  capacity,
		items:     make(map[string]*list.Element, capacity), // Pre-allocate map hint
		evictList: list.New(),
	}
}

// Get retrieves a value, moving the item to the front (most recently used).
func (c *LRUCache) Get(key string) (string, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if elem, hit := c.items[key]; hit {
		c.evictList.MoveToFront(elem) // Mark as recently used
		// Type assertion needed as list stores interface{}
		return elem.Value.(*entry).value, true
	}
	return "", false
}

// Put inserts or updates a value, moving/adding it to the front. Evicts if needed.
func (c *LRUCache) Put(key, value string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if key exists - Update value and move to front
	if elem, hit := c.items[key]; hit {
		c.evictList.MoveToFront(elem)
		elem.Value.(*entry).value = value // Update the value
		return
	}

	// Key doesn't exist - Add new entry

	// Check for capacity and evict LRU item if full
	if c.evictList.Len() >= c.capacity {
		c.removeOldest()
	}

	// Add the new item
	newEntry := &entry{key, value}
	element := c.evictList.PushFront(newEntry)
	c.items[key] = element
}

// removeOldest removes the least recently used item from the cache.
// MUST be called with the mutex held.
func (c *LRUCache) removeOldest() {
	elem := c.evictList.Back() // Get the last element (LRU)
	if elem != nil {
		entryToRemove := c.evictList.Remove(elem).(*entry) // Remove from list
		delete(c.items, entryToRemove.key)                 // Remove from map
	}
}

// --- Sharded Cache Implementation ---

// ShardedCache manages multiple LRUCache shards.
type ShardedCache struct {
	shards []*LRUCache
}

// NewShardedCache creates and initializes all cache shards.
func NewShardedCache(numShards, capacityPerShard int) *ShardedCache {
	if numShards <= 0 {
		numShards = NumShards // Default
	}
	shards := make([]*LRUCache, numShards)
	for i := 0; i < numShards; i++ {
		shards[i] = NewLRUCache(capacityPerShard)
	}
	log.Printf("Initialized sharded cache with %d shards, %d capacity per shard (Total Capacity: %d)",
		numShards, capacityPerShard, numShards*capacityPerShard)
	return &ShardedCache{shards: shards}
}

// getShardIndex calculates the shard index for a given key.
func (sc *ShardedCache) getShardIndex(key string) int {
	hasher := fnv.New32a()
	hasher.Write([]byte(key))
	// Use modulo. If NumShards is a power of 2, `hash & (NumShards - 1)` is faster.
	return int(hasher.Sum32()) % len(sc.shards)
	// Example for power of 2: return int(hasher.Sum32() & (uint32(len(sc.shards)) - 1))
}

// Get retrieves a value from the appropriate shard.
func (sc *ShardedCache) Get(key string) (string, bool) {
	shardIndex := sc.getShardIndex(key)
	shard := sc.shards[shardIndex]
	return shard.Get(key) // Delegate to the specific shard's Get method
}

// Put inserts/updates a value into the appropriate shard.
func (sc *ShardedCache) Put(key, value string) {
	shardIndex := sc.getShardIndex(key)
	shard := sc.shards[shardIndex]
	shard.Put(key, value) // Delegate to the specific shard's Put method
}

// writeJSONError sends a standardized JSON error response.
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(GenericErrorResponse{
		Status:  "ERROR",
		Message: message,
	})
}

// --- HTTP Handlers --- (Updated to use ShardedCache)
func HandlePut(cache *ShardedCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PutRequest

		// Limit request body size
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB limit

		// Decode request body
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			msg := "Invalid JSON format."
			if strings.Contains(err.Error(), "http: request body too large") {
				msg = "Request body exceeds limit (1MB)."
			}
			writeJSONError(w, msg, http.StatusBadRequest)
			return
		}

		// Validate Key (must exist and check length using rune count for UTF-8)
		key := strings.TrimSpace(req.Key)
		if key == "" {
			writeJSONError(w, "Key cannot be empty.", http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(key) > MaxKeyLength {
			writeJSONError(w, fmt.Sprintf("Key exceeds maximum length (%d characters).", MaxKeyLength), http.StatusBadRequest)
			return
		}

		// Validate Value (check length using rune count for UTF-8)
		// Assuming value can be empty, but not exceed max length. Adjust if empty value is disallowed.
		if utf8.RuneCountInString(req.Value) > MaxValueLength {
			writeJSONError(w, fmt.Sprintf("Value exceeds maximum length (%d characters).", MaxValueLength), http.StatusBadRequest)
			return
		}

		// Store the key-value pair
		cache.Put(key, req.Value) // Use the trimmed key

		// Send success response
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(PutSuccessResponse{
			Status:  "OK",
			Message: "Key inserted/updated successfully.",
		})
	}
}

func HandleGet(cache *ShardedCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		key = strings.TrimSpace(key) // Trim whitespace

		// Validate key presence
		if key == "" {
			writeJSONError(w, "Missing 'key' query parameter.", http.StatusBadRequest)
			return
		}

		// Validate key length (using rune count for UTF-8)
		if utf8.RuneCountInString(key) > MaxKeyLength {
			writeJSONError(w, fmt.Sprintf("Key exceeds maximum length (%d characters).", MaxKeyLength), http.StatusBadRequest)
			return
		}

		// Attempt to retrieve the value
		value, found := cache.Get(key)

		// Handle Key Not Found
		if !found {
			writeJSONError(w, "Key not found.", http.StatusNotFound)
			return
		}

		// Handle Success (Key Found)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(GetSuccessResponse{
			Status: "OK",
			Key:    key,
			Value:  value,
		})
	}
}


// --- Main Function ---
func main() {
	// Initialize the sharded cache
	kvCache := NewShardedCache(NumShards, MaxCapacityPerShard)
	if kvCache == nil {
		log.Fatal("Failed to initialize sharded cache")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/put", HandlePut(kvCache))
	mux.HandleFunc("/get", HandleGet(kvCache))

	// Add a simple health check endpoint (good practice)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})


	serverAddr := "0.0.0.0:7171"
	log.Printf("Starting key-value cache server on %s...", serverAddr)


    // Using default timeouts for simplicity here:
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}