package runtime

import (
	"fmt"
	"sync"
)

// Concurrency Semaphores
var (
	semaphores   = make(map[string]chan struct{})
	semaphoresMu sync.Mutex
)

// Rate Limiting Semaphores
func AcquireSemaphore(id string, limit int) {
	semaphoresMu.Lock()
	sem, exists := semaphores[id]
	if !exists {
		sem = make(chan struct{}, limit)
		semaphores[id] = sem
	}
	semaphoresMu.Unlock()

	sem <- struct{}{}
}

func ReleaseSemaphore(id string) {
	semaphoresMu.Lock()
	sem, exists := semaphores[id]
	semaphoresMu.Unlock()
	if exists {
		<-sem
	}
}

// Await runs a function asynchronously and blocks until it returns.
func Await(fn func() interface{}) interface{} {
	ch := make(chan interface{}, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- nil
			}
		}()
		ch <- fn()
	}()
	return <-ch
}

// AwaitAll runs multiple functions concurrently and returns all results as []interface{}.
func AwaitAll(fns []func() interface{}) interface{} {
	results := make([]interface{}, len(fns))
	var wg sync.WaitGroup
	wg.Add(len(fns))
	for i, fn := range fns {
		go func(idx int, f func() interface{}) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx] = nil
				}
			}()
			results[idx] = f()
		}(i, fn)
	}
	wg.Wait()
	return results
}

// Atomic operations — thread-safe counters and values

type AtomicValue struct {
	mu    sync.RWMutex
	value interface{}
}

var (
	atomicValues   = make(map[string]*AtomicValue)
	atomicValuesMu sync.Mutex
)

func getOrCreateAtomic(name string) *AtomicValue {
	atomicValuesMu.Lock()
	defer atomicValuesMu.Unlock()
	if av, ok := atomicValues[name]; ok {
		return av
	}
	av := &AtomicValue{value: 0}
	atomicValues[name] = av
	return av
}

// AtomicNew creates a new atomic value with an initial value.
func AtomicNew(name interface{}, initial interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	av.value = initial
	av.mu.Unlock()
	return nil
}

// AtomicInc increments an atomic counter by 1 and returns the new value.
func AtomicInc(name interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	defer av.mu.Unlock()
	switch v := av.value.(type) {
	case int:
		av.value = v + 1
		return av.value
	case int64:
		av.value = v + 1
		return av.value
	default:
		av.value = 1
		return 1
	}
}

// AtomicDec decrements an atomic counter by 1 and returns the new value.
func AtomicDec(name interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	defer av.mu.Unlock()
	switch v := av.value.(type) {
	case int:
		av.value = v - 1
		return av.value
	case int64:
		av.value = v - 1
		return av.value
	default:
		av.value = -1
		return -1
	}
}

// AtomicGet returns the current value of an atomic.
func AtomicGet(name interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.RLock()
	defer av.mu.RUnlock()
	return av.value
}

// AtomicSet sets the value of an atomic.
func AtomicSet(name interface{}, value interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	av.value = value
	av.mu.Unlock()
	return nil
}

// AtomicCAS performs a compare-and-swap. Returns true if swapped.
func AtomicCAS(name interface{}, expected interface{}, newValue interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	defer av.mu.Unlock()
	if fmt.Sprint(av.value) == fmt.Sprint(expected) {
		av.value = newValue
		return true
	}
	return false
}

// Channel operations — Go channels exposed to Serv

// ChannelNew creates a buffered channel with the given capacity.
// Usage: let ch = channel.new(100)
func ChannelNew(capacity interface{}) interface{} {
	cap := toInt(capacity)
	if cap <= 0 {
		cap = 1
	}
	return make(chan interface{}, cap)
}

// ChannelSend sends a value to a channel. Blocks if channel is full.
// Usage: channel.send(ch, value)
func ChannelSend(ch interface{}, value interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		c <- value
	}
	return nil
}

// ChannelReceive receives a value from a channel. Blocks until a value is available.
// Usage: let value = channel.receive(ch)
func ChannelReceive(ch interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		val, ok := <-c
		if !ok {
			return nil // channel closed
		}
		return val
	}
	return nil
}

// ChannelTryReceive attempts to receive without blocking. Returns nil if nothing available.
// Usage: let value = channel.tryReceive(ch)
func ChannelTryReceive(ch interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		select {
		case val, ok := <-c:
			if !ok {
				return nil
			}
			return val
		default:
			return nil
		}
	}
	return nil
}

// ChannelTrySend attempts to send without blocking. Returns true if sent, false if full.
// Usage: let sent = channel.trySend(ch, value)
func ChannelTrySend(ch interface{}, value interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		select {
		case c <- value:
			return true
		default:
			return false
		}
	}
	return false
}

// ChannelClose closes a channel. Receivers will get nil after all buffered values are consumed.
// Usage: channel.close(ch)
func ChannelClose(ch interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		close(c)
	}
	return nil
}

// ChannelLen returns the number of elements currently buffered in the channel.
// Usage: let pending = channel.len(ch)
func ChannelLen(ch interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		return len(c)
	}
	return 0
}
