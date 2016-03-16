package cache

import (
	"container/list"
	"github.com/biogo/store/llrb"
	"github.com/biogo/store/interval"
	log "github.com/Sirupsen/logrus"
	"sync/atomic"
)

type EvictionPolicy int

const (
	CacheLRU EvictionPolicy = iota
	CacheFIFO
	CacheNone
)

type Config struct {
	Policy EvictionPolicy
	// ShouldEvict is the callback to check whether the entry should be evicted.
	// The arguments are the size of the cache and entry's key-value.
	ShouldEvict func(size int, key, value interface{}) bool
	// OnEvict is the callback that'll be called when an entry is evicted.
	OnEvict func(key, value interface{})
}

type entry struct {
	key, value interface{}
	le *list.Element
}

// Compare implements the Compare interface.
// We move the interface requirement to the entry key.
func (e *entry) Compare(b llrb.Comparable) int {
	return e.key.(llrb.Comparable).Compare(b.(*entry).key.(llrb.Comparable))
}

func (e *entry) ID() uintptr {
	return e.key.(*IntervalKey).id
}

func (e *entry) NewMutable() interval.Mutable {
	ik := e.key.(*IntervalKey)
	return &IntervalKey{id: ik.id, start: ik.start, end: ik.end}
}

func (e *entry) Start() interval.Comparable {
	return e.key.(*IntervalKey).start
}

func (e *entry) End() interval.Comparable {
	return e.key.(*IntervalKey).end
}

func (e *entry) Overlap(r interval.Range) bool {
	return e.key.(*IntervalKey).Overlap(r)
}

// storage is the interface for the backing data struct used for the cache.
type storage interface {
	put(e *entry)
	get(key interface{}) *entry
	del(key interface{})
	size() int
	clear()
}

type baseCache struct {
	cfg *Config
	storage storage
	ll *list.List
}

func newBaseCache(cfg *Config, storage storage) *baseCache {
	return &baseCache{
		cfg: cfg,
		storage: storage,
		ll: list.New(),
	}
}

func(bc *baseCache) Put(key, value interface{}) {
	if e := bc.storage.get(key); e != nil {
		bc.use(e)
		return
	}
	e := &entry{key: key, value: value}
	if bc.cfg.Policy != CacheNone {
		e.le = bc.ll.PushFront(e)
	}
	bc.storage.put(e)

	if bc.cfg.ShouldEvict != nil && bc.cfg.Policy != CacheNone {
		for {
			if bc.storage.size() <= 0 {
				break
			}
			e := bc.ll.Back().Value.(*entry)
			if bc.cfg.ShouldEvict(bc.storage.size(), e.key, e.value) {
				bc.removeEntry(e)
			} else {
				break
			}
		}
	}
}

func (bc *baseCache) use(e *entry) {
	if bc.cfg.Policy == CacheLRU {
		bc.ll.MoveToFront(e.le)
	}
}

func (bc *baseCache) Get(key interface{}) interface{} {
	e := bc.storage.get(key)
	if e != nil {
		bc.use(e)
		return e.value
	}
	return nil
}

func (bc *baseCache) Del(key interface{}) {
	if e := bc.storage.get(key); e != nil {
		bc.removeEntry(e)
	}
}

// Clear clears all entries in the cache.
func (bc *baseCache) Clear() {
	bc.storage.clear()
}

func (bc *baseCache) removeEntry(e *entry) {
	if bc.cfg.Policy != CacheNone {
		bc.ll.Remove(e.le)
	}
	bc.storage.del(e.key)
	if bc.cfg.OnEvict != nil {
		bc.cfg.OnEvict(e.key, e.value)
	}
}

func (bc *baseCache) Size() int {
	return bc.storage.size()
}

type UnorderedCache struct {
	*baseCache
	hash map[interface{}]*entry
}

func NewUnorderedCache(cfg *Config) *UnorderedCache {
	uc := &UnorderedCache{hash: make(map[interface{}]*entry)}
	bc := newBaseCache(cfg, uc)
	uc.baseCache = bc
	return uc
}

func (uc *UnorderedCache) put(e *entry) {
	uc.hash[e.key] = e
}

func (uc *UnorderedCache) get(key interface{}) *entry {
	if v, ok := uc.hash[key]; ok {
		return v
	}
	return nil
}

func (uc *UnorderedCache) del(key interface{}) {
	delete(uc.hash, key)
}

func (uc *UnorderedCache) clear() {
	for k := range uc.hash {
		uc.del(k)
	}
}

func (uc *UnorderedCache) size() int {
	return len(uc.hash)
}

type OrderedCache struct {
	*baseCache
	llrb llrb.Tree
}

func NewOrderedCache(cfg *Config) *OrderedCache {
	oc := &OrderedCache{llrb: llrb.Tree{}}
	bc := newBaseCache(cfg, oc)
	oc.baseCache = bc
	return oc
}

func (oc *OrderedCache) put(e *entry) {
	oc.llrb.Insert(e)
}

func (oc *OrderedCache) get(key interface{}) *entry {
	if e, ok := oc.llrb.Get(&entry{key: key}).(*entry); ok {
		return e
	}
	return nil
}

func (oc *OrderedCache) del(key interface{}) {
	oc.llrb.Delete(&entry{key: key})
}

func (oc *OrderedCache) clear() {
	oc.llrb.Do(func(e llrb.Comparable) bool {
		oc.del(e.(*entry).key)
		return false
	})
}

func (oc *OrderedCache) size() int {
	return oc.llrb.Len()
}

func (oc *OrderedCache) Do(f func(key, value interface{}) bool) {
	oc.llrb.Do(func(e llrb.Comparable) bool {
		ent := e.(*entry)
		oc.use(ent)
		return f(ent.key, ent.value)
	})
}

func (oc *OrderedCache) DoRange(f func(key, value interface{}) bool, from, to interface{}) {
	oc.llrb.DoRange(func(e llrb.Comparable) bool {
		ent := e.(*entry)
		oc.use(ent)
		return f(ent.key, ent.value)
	}, &entry{key: from}, &entry{key: to})
}

func (oc *OrderedCache) Floor(key interface{}) (interface{}, interface{}) {
	if e, ok := oc.llrb.Floor(&entry{key: key}).(*entry); ok {
		return e.key, e.value
	}
	return nil, nil
}

func (oc *OrderedCache) Ceil(key interface{}) (interface{}, interface{}) {
	if e, ok := oc.llrb.Ceil(&entry{key: key}).(*entry); ok {
		return e.key, e.value
	}
	return nil, nil
}

type IntervalKey struct {
	id uintptr
	start, end interval.Comparable
}

// interval.Overlapper interface
func (ik *IntervalKey) Overlap(r interval.Range) bool {
	return ik.start.Compare(r.(*IntervalKey).end) < 0 && ik.end.Compare(r.(*IntervalKey).start) > 0
}

// interval.Range interface
func (ik *IntervalKey) Start() interval.Comparable { return ik.start }

func (ik *IntervalKey) End() interval.Comparable { return ik.end }

// interval.Mutable interface
func (ik *IntervalKey) SetStart(start interval.Comparable) {
	ik.start = start
}

func (ik *IntervalKey) SetEnd(end interval.Comparable) {
	ik.end = end
}

var intervalKeyIdAllocator int64 = 1
func (ic *IntervalCache) NewKey(start, end interval.Comparable) *IntervalKey {
	return &IntervalKey{id: uintptr(
		atomic.AddInt64(&intervalKeyIdAllocator, 1)),
		start: start, end: end,
	}
}

type IntervalCache struct {
	*baseCache
	interval interval.Tree
}

func NewIntervalCache(cfg *Config) *IntervalCache {
	ic := &IntervalCache{interval: interval.Tree{}}
	bc := newBaseCache(cfg, ic)
	ic.baseCache = bc
	return ic
}

func (ic *IntervalCache) put(e *entry) {
	err := ic.interval.Insert(e, false)
	if err != nil {
		log.Errorf("Error when put entry into interval cache: %v", err)
	}
}

func (ic *IntervalCache) get(key interface{}) *entry {
	ik := key.(*IntervalKey)
	if es := ic.interval.Get(ik); len(es) > 0 {
		for _, e := range es {
			if e.ID() == ik.id {
				return e.(*entry)
			}
		}
	}
	return nil
}

func (ic *IntervalCache) del(key interface{}) {
	err := ic.interval.Delete(&entry{key: key}, false)
	if err != nil {
		log.Errorf("Error when delete entry from interval cache: %v", err)
	}
}

func (ic *IntervalCache) clear() {
	ic.interval.Do(func(e interval.Interface) bool {
		ic.del(e.(*entry).key)
		return false
	})
}

func (ic *IntervalCache) size() int {
	return ic.interval.Len()
}

type Overlap struct {
	Key *IntervalKey
	Value interface{}
}


func (ic *IntervalCache) GetOverlap(start, end interval.Comparable) []Overlap {
	es := ic.interval.Get(ic.NewKey(start, end))
	ret := make([]Overlap, len(es))
	for i, e := range es {
		ent := e.(*entry)
		ic.use(ent)
		ret[i].Key = ent.key.(*IntervalKey)
		ret[i].Value = ent.value
	}
	return ret
}

func (ic *IntervalCache) Do(f func(key, value interface{}) bool) {
	ic.interval.Do(func(e interval.Interface) bool {
		ent := e.(*entry)
		ic.use(ent)
		return f(ent.key, ent.value)
	})
}