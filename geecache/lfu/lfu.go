package lfu

import (
	"container/heap"
	"log"
	"time"
)

type LFUCache struct {
	maxBytes int64 // 最大存储容量
	nBytes   int64 // 已占用的容量
	// 使用一个 heap 来管理缓存项，heap 中的元素按照频率排序
	// heap 实现了一个最小堆，即堆顶元素是最小值
	heap       *entryHeap
	cache      map[string]*entry             // map，键是字符串，值是堆中对应节点的指针
	OnEvicted  func(key string, value Value) // OnEvicted 是某条记录被移除时的回调函数，可以为 nil
	defaultTTL time.Duration
}

type Value interface {
	Len() int
} // 为了通用性，我们允许值是实现了 Value 接口的任意类型，该接口只包含了一个方法 Len() int，用于返回值所占用的内存大小。

type entry struct {
	key    string
	value  Value
	freq   int // 记录访问频率
	index  int // 在堆中的索引，用于快速定位
	expire time.Time
}

// entryHeap 实现了 heap.Interface 接口，用于对 entry 进行堆排序
type entryHeap []*entry

func (h entryHeap) Len() int {
	return len(h)
}

func (h entryHeap) Less(i, j int) bool {
	// 小于号是因为我们需要一个最小堆
	return h[i].freq < h[j].freq
}

func (h entryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *entryHeap) Push(x interface{}) {
	entry := x.(*entry)
	entry.index = len(*h)
	*h = append(*h, entry)
}

func (h *entryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	entry := old[n-1]
	entry.index = -1 // for safety
	*h = old[0 : n-1]
	return entry
}

func New(maxBytes int64, onEvicted func(string, Value), defaultTTL time.Duration) *LFUCache {
	return &LFUCache{
		maxBytes:   maxBytes,
		heap:       &entryHeap{},
		cache:      make(map[string]*entry),
		OnEvicted:  onEvicted,
		defaultTTL: defaultTTL,
	}
}

func (c *LFUCache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok {
		if ele.expire.Before(time.Now()) {
			c.removeElement(ele)
			log.Printf("The LFUcache key—%s has expired", key)
			return nil, false
		}
		ele.freq++
		heap.Fix(c.heap, ele.index)
		return ele.value, true
	}
	return
}

func (c *LFUCache) RemoveOldest() {
	entry := heap.Pop(c.heap).(*entry)
	delete(c.cache, entry.key)
	c.nBytes -= int64(len(entry.key)) + int64(entry.value.Len())
	if c.OnEvicted != nil {
		c.OnEvicted(entry.key, entry.value)
	}
}

func (c *LFUCache) Add(key string, value Value, ttl time.Duration) {
	if ele, ok := c.cache[key]; ok {
		ele.freq++
		ele.value = value
		ele.expire = time.Now().Add(ttl)
		heap.Fix(c.heap, ele.index)
	} else {
		entry := &entry{
			key:    key,
			value:  value,
			freq:   1,
			expire: time.Now().Add(ttl),
		}
		heap.Push(c.heap, entry)
		c.cache[key] = entry
		c.nBytes += int64(len(key)) + int64(value.Len())
	}

	for c.maxBytes != 0 && c.maxBytes < c.nBytes {
		c.RemoveOldest()
	}
}

func (c *LFUCache) Len() int {
	return len(c.cache)
}

func (c *LFUCache) removeElement(e *entry) {
	heap.Remove(c.heap, e.index)
	delete(c.cache, e.key)
	c.nBytes -= int64(len(e.key)) + int64(e.value.Len())
	if c.OnEvicted != nil {
		c.OnEvicted(e.key, e.value)
	}
}
