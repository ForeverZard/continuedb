package cache

import (
	"testing"
	"fmt"
	"github.com/biogo/store/llrb"
	"bytes"
	"strconv"
)

type key string
type value string
type keyvalue struct {
	key key
	value value
}

func (k key) Compare(b llrb.Comparable) int {
	return bytes.Compare([]byte(k), []byte(b.(key)))
}

var testCases = []struct{
	put *keyvalue
	del key
	get key
	expect value
}{
	{nil, "k1", "k1", ""},
	{&keyvalue{"k1", "v1"}, "", "k1", "v1"},
	{&keyvalue{"k1", "v1"}, "", "k2", ""},
	{&keyvalue{"k1", "v1"}, "k1", "k1", ""},
}

var cfg = &Config{
	Policy: CacheLRU,
	ShouldEvict: func(size int, key, value interface{}) bool {
		if size > 4 {
			return true
		}
		return false
	},
	OnEvict: func(key, value interface{}) {
		fmt.Printf("Evict entry{key: %v, value: %v}\n", key, value)
	},
}

func TestUnorderedCache(t *testing.T) {
	uc := NewUnorderedCache(cfg)
	for _, cs := range testCases {
		if cs.put != nil {
			uc.Put(cs.put.key, cs.put.value)
			t.Logf("cache size: %v", uc.Size())
		}
		if len(cs.del) > 0 {
			uc.Del(cs.del)
		}
		if real := uc.Get(cs.get); cs.expect != "" && cs.expect != real {
			t.Fatalf("Expect %v, got %v", cs.expect, real)
		}
		uc.Clear()
	}
}

func TestOrderedCache(t *testing.T) {
	oc := NewOrderedCache(cfg)
	for _, cs := range testCases {
		if cs.put != nil {
			oc.Put(cs.put.key, cs.put.value)
			t.Logf("cache size: %v\n", oc.Size())
		}
		if len(cs.del) > 0 {
			oc.Del(cs.del)
		}
		if real := oc.Get(cs.get); cs.expect != "" && cs.expect != real {
			t.Fatalf("Expect %v, got %v", cs.expect, real)
		}
		oc.Clear()
	}
	for i := 1; i <= 4; i++ {
		var key key = key("k" + strconv.Itoa(i))
		oc.Put(key, i)
	}
	oc.DoRange(func(key, value interface{}) bool {
		if 2 != value.(int) {
			t.Fatal()
		}
		return false
	}, key("k" + strconv.Itoa(2)), key("k" + strconv.Itoa(3)))
	var cnt = 0
	oc.Do(func(key, value interface{}) bool {
		cnt++
		return false
	})
	if cnt != 4 {
		t.Fatal()
	}
	fk, _ := oc.Floor(key("k2"))
	if fk != key("k2") {
		t.Fatalf("fk = %v", fk)
	}
	fk2, _ := oc.Floor(key("k0"))
	if fk2 != nil {
		t.Fatal()
	}
	ck, _ := oc.Ceil(key("k3"))
	if ck != key("k3") {
		t.Fatal("ck = %v", ck)
	}
	ck2, _ := oc.Ceil(key("k5"))
	if ck2 != nil {
		t.Fatal()
	}
}

func TestIntervalCache(t *testing.T) {
	// TODO
}
