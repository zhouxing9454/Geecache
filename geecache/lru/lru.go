package lru

import (
	"container/list"
)

type LRUCache struct {
	maxBytes  int64                         // 最大存储容量
	nBytes    int64                         // 已占用的容量
	ll        *list.List                    //直接使用 Go 语言标准库实现的双向链表list.List
	cache     map[string]*list.Element      //map,键是字符串，值是双向链表中对应节点的指针
	OnEvicted func(key string, value Value) //OnEvicted 是某条记录被移除时的回调函数，可以为 nil
} // Cache is a LRU cache. It is not safe for concurrent access.

type entry struct {
	key   string
	value Value
} // 键值对 entry 是双向链表节点的数据类型，在链表中仍保存每个值对应的 key 的好处在于，淘汰队首节点时，需要用 key 从字典中删除对应的映射。

type Value interface {
	Len() int
} // 为了通用性，我们允许值是实现了 Value 接口的任意类型，该接口只包含了一个方法 Len() int，用于返回值所占用的内存大小。

func New(maxBytes int64, onEvicted func(string, Value)) *LRUCache {
	return &LRUCache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

func (c *LRUCache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		return kv.value, true
	}
	return
} //Get 方法用于根据键获取缓存中的值。如果键存在，则将对应的节点移动到链表的最前面（表示最近使用），并返回对应的值和 true；如果键不存在，则返回零值和 false。

func (c *LRUCache) RemoveOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		delete(c.cache, kv.key)
		c.nBytes -= int64(len(kv.key)) + int64(kv.value.Len())
		if c.OnEvicted != nil {
			c.OnEvicted(kv.key, kv.value)
		}
	}
} //RemoveOldest 方法用于移除最久未使用的记录（即链表中的最后一个节点）。它会从链表和哈希表中删除对应的节点，并更新已占用的容量。如果设置了 OnEvicted 回调函数，会在移除记录后调用该函数。

func (c *LRUCache) Add(key string, value Value) {
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		c.nBytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
	} else {
		ele = c.ll.PushFront(&entry{key: key, value: value})
		c.cache[key] = ele
		c.nBytes += int64(len(key)) + int64(value.Len())
	}
	for c.maxBytes != 0 && c.maxBytes < c.nBytes {
		c.RemoveOldest()
	}
} //Add 方法用于向缓存中添加新的键值对。如果键已存在，则更新对应的值，并将节点移动到链表的最前面；如果键不存在，则在链表头部插入新的节点，并更新已占用的容量。如果添加新的键值对后超出了最大存储容量，则会连续移除最久未使用的记录，直到满足容量要求。

func (c *LRUCache) Len() int {
	return c.ll.Len()
} //Len 方法返回当前缓存中的记录数量。

//为什么使用这些数据结构：
//ll（双向链表）：用于维护记录的访问顺序。当访问或添加某个键值对时，可以将对应节点移动到链表的最前面，表示最近使用。当需要移除最久未使用的节点时，可以直接操作链表的尾部节点。
//cache（哈希表）：用于快速查找键对应的双向链表中的节点。通过在哈希表中存储键和对应节点的指针，可以在 O(1) 的时间复杂度内查找到节点，从而实现快速访问和更新操作。
//entry（节点）：双向链表中的节点，保存了键值对。在链表中仍保存每个值对应的键的好处在于，当需要移除队首节点时，可以从字典中删除对应的键，以保持缓存和哈希表的一致性。
