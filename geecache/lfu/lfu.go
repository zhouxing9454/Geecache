package lfu

import (
	"container/heap"
	"log"
	"time"
)

/*
LFUCache 定义了一个结构体，用来实现lfu缓存淘汰算法
maxBytes：最大存储容量
nBytes：已占用的容量
heap：使用一个 heap 来管理缓存项，heap 中的元素按照频率排序(heap实现了一个最小堆，即堆顶元素是最小值)
cache：map，键是字符串，值是堆中对应节点的指针
OnEvicted：是某条记录被移除时的回调函数，可以为 nil
defaultTTL：记录在缓存中的默认过期时间
*/
type LFUCache struct {
	maxBytes   int64
	nBytes     int64
	heap       *entryHeap
	cache      map[string]*entry
	OnEvicted  func(key string, value Value)
	defaultTTL time.Duration
}

type Value interface {
	Len() int
} // 为了通用性，我们允许值是实现了 Value 接口的任意类型，该接口只包含了一个方法 Len() int，用于返回值所占用的内存大小。

type entry struct {
	key    string
	value  Value
	freq   int       // 记录访问频率
	index  int       // 在堆中的索引，用于快速定位
	expire time.Time //节点的过期时间
}

// entryHeap 实现了 heap.Interface 接口，用于对 entry 进行堆排序,实现最小堆
type entryHeap []*entry

// Len 函数用于返回entryHeap的长度
func (h entryHeap) Len() int {
	return len(h)
}

// Less 函数实现最小堆的排序
func (h entryHeap) Less(i, j int) bool {
	//小于号是因为我们需要一个最小堆
	return h[i].freq < h[j].freq
}

// Swap 函数交换缓存项，包括在堆中的索引
func (h entryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

// Push 函数用于插入一个缓存项
func (h *entryHeap) Push(x interface{}) {
	entry := x.(*entry)
	entry.index = len(*h)
	*h = append(*h, entry)
}

// Pop 函数用于删除一个缓存项
func (h *entryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	entry := old[n-1]
	entry.index = -1 // for safety
	*h = old[0 : n-1]
	return entry
}

// New 函数通过传入maxBytes,onEvicted,defaultTTL这些参数，返回一个LFUCache结构体。
func New(maxBytes int64, onEvicted func(string, Value), defaultTTL time.Duration) *LFUCache {
	return &LFUCache{
		maxBytes:   maxBytes,
		heap:       &entryHeap{},
		cache:      make(map[string]*entry),
		OnEvicted:  onEvicted,
		defaultTTL: defaultTTL,
	}
}

// Get 函数用于根据键获取缓存中的值。如果键存在，则将对应的节点的freq频率增加、调用Fix函数维持堆的性质，并返回对应的值和 true；如果键不存在或者键已经过期，则返回零值和 false。
func (c *LFUCache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok {
		if ele.expire.Before(time.Now()) {
			c.removeElement(ele)
			log.Printf("The LFUcache key—%s has expired", key)
			return nil, false
		}
		ele.freq++
		heap.Fix(c.heap, ele.index)
		//Fix 方法用于在索引 index 处的元素值发生变化后重新确立堆的顺序。在索引 index 的元素值发生改变后，调用 Fix 方法可以保持堆的性质。
		//Fix 方法的时间复杂度是 O(log n)，其中 n = h.Len() 表示堆中元素的数量。
		return ele.value, true
	}
	return
}

// RemoveOldest 函数删除频率最低的缓存项。
func (c *LFUCache) RemoveOldest() {
	entry := heap.Pop(c.heap).(*entry)
	delete(c.cache, entry.key)
	c.nBytes -= int64(len(entry.key)) + int64(entry.value.Len())
	if c.OnEvicted != nil {
		c.OnEvicted(entry.key, entry.value)
	}
}

// Add 函数用于插入一个缓存项。
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

// Len 方法返回当前缓存中的记录数量。
func (c *LFUCache) Len() int {
	return len(c.cache)
}

// removeElement 函数删除传入的缓存项。
func (c *LFUCache) removeElement(e *entry) {
	heap.Remove(c.heap, e.index)
	delete(c.cache, e.key)
	c.nBytes -= int64(len(e.key)) + int64(e.value.Len())
	if c.OnEvicted != nil {
		c.OnEvicted(e.key, e.value)
	}
}
