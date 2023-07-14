package lru

import (
	"container/list"
)

// Cache is a LRU cache. It is not safe for concurrent access.
type Cache struct {
	maxBytes  int64                         // 最大存储容量
	nBytes    int64                         // 已占用的容量
	ll        *list.List                    //直接使用 Go 语言标准库实现的双向链表list.List
	cache     map[string]*list.Element      //map,键是字符串，值是双向链表中对应节点的指针
	OnEvicted func(key string, value Value) //OnEvicted 是某条记录被移除时的回调函数，可以为 nil
}

// 键值对 entry 是双向链表节点的数据类型，在链表中仍保存每个值对应的 key 的好处在于，淘汰队首节点时，需要用 key 从字典中删除对应的映射。
type entry struct {
	key   string
	value Value
}

type Value interface { // 为了通用性，我们允许值是实现了 Value 接口的任意类型，该接口只包含了一个方法 Len() int，用于返回值所占用的内存大小。
	Len() int
}

func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

func (c *Cache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		return kv.value, true
	}
	return
}
