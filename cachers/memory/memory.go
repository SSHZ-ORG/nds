// Package memory IS NOT MEANT TO BE USED - THIS IS FOR PROOF OF CONCEPT AND TESTING ONLY, IT
// IS A LOCAL MEMORY STORE AND WILL RESULT IN INCONSISTENT CACHING FOR DSITRIBUTED SYSTEMS!
package memory

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"

	"github.com/qedus/nds"
)

// NewCacher will intiialize a new in-memory cache
// and return a nds.Cacher using that cache.
func NewCacher() nds.Cacher {
	store := cache.New(32*time.Second, 10*time.Minute)
	return &memory{store: store}
}

type object struct {
	flags uint32
	value []byte
}

type memory struct {
	store *cache.Cache
	sync.RWMutex
}

func (m *memory) NewContext(c context.Context) (context.Context, error) {
	return c, nil
}

func (m *memory) AddMulti(_ context.Context, items []*nds.Item) error {
	m.RLock()
	defer m.RUnlock()
	me := make(nds.MultiError, len(items))
	hasErr := false
	for i, item := range items {
		if err := m.store.Add(item.Key, &object{flags: item.Flags, value: append([]byte(nil), item.Value...)}, item.Expiration); err != nil {
			me[i] = nds.ErrNotStored
			hasErr = true
		}
	}
	if hasErr {
		return me
	}
	return nil
}

func (m *memory) CompareAndSwapMulti(_ context.Context, items []*nds.Item) error {
	m.Lock() // No other cache operations should happen while we do our CAS operations, here to make the ops "atomic"
	defer m.Unlock()
	me := make(nds.MultiError, len(items))
	hasErr := false
	for i, item := range items {
		if cacheItem, found := m.store.Get(item.Key); found {
			obj := cacheItem.(*object)
			ndsItem := &nds.Item{
				Flags: obj.flags,
				Value: append([]byte(nil), obj.value...),
			}
			hasher := sha1.New()
			binary.Write(hasher, binary.LittleEndian, ndsItem.Flags)
			hasher.Write(ndsItem.Value) // err is always nil
			if bytes.Compare(item.GetCASInfo().([]byte), hasher.Sum(nil)) == 0 {
				m.store.Set(item.Key, &object{flags: item.Flags, value: append([]byte(nil), item.Value...)}, item.Expiration)
			} else {
				hasErr = true
				me[i] = nds.ErrCASConflict
			}
		} else {
			hasErr = true
			me[i] = nds.ErrNotStored
		}
	}
	if hasErr {
		return me
	}
	return nil
}

func (m *memory) DeleteMulti(_ context.Context, keys []string) error {
	m.RLock()
	defer m.RUnlock()
	for _, key := range keys {
		m.store.Delete(key)
	}
	return nil
}

func (m *memory) GetMulti(_ context.Context, keys []string) (map[string]*nds.Item, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	result := make(map[string]*nds.Item)

	for _, key := range keys {
		if cacheItem, found := m.store.Get(key); found {
			obj := cacheItem.(*object)
			ndsItem := &nds.Item{
				Key:   key,
				Flags: obj.flags,
				Value: append([]byte(nil), obj.value...),
			}
			hasher := sha1.New()
			binary.Write(hasher, binary.LittleEndian, ndsItem.Flags)
			hasher.Write(ndsItem.Value)
			ndsItem.SetCASInfo(hasher.Sum(nil))
			result[key] = ndsItem
		}
	}

	return result, nil
}

func (m *memory) SetMulti(_ context.Context, items []*nds.Item) error {
	m.RLock()
	defer m.RUnlock()
	for _, item := range items {
		m.store.Set(item.Key, &object{flags: item.Flags, value: append([]byte(nil), item.Value...)}, item.Expiration)
	}
	return nil
}