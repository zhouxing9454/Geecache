package geecache

import (
	"Geecache/geecache/lfu"
	"Geecache/geecache/lru"
	"sync"
	"time"
)

// BaseCache 是一个接口，定义了基本的缓存操作方法。它包含了两个方法：add 和 get，用于向缓存中添加数据和从缓存中获取数据。
type BaseCache interface {
	add(key string, value ByteView)
	get(key string) (value ByteView, ok bool)
}

// LRUcache 的实现非常简单，实例化 lru，封装 get 和 add 方法。
type LRUcache struct {
	mu         sync.RWMutex // 读写锁
	lru        *lru.LRUCache
	cacheBytes int64         // lru的maxBytes
	ttl        time.Duration // lru的defaultTTL
}

// add 函数用于向缓存中添加数据
func (c *LRUcache) add(key string, value ByteView) {
	c.mu.Lock() //写锁
	defer c.mu.Unlock()
	if c.lru == nil {
		c.lru = lru.New(c.cacheBytes, nil, c.ttl)
	}
	/*
		判断c.lru 是否为 nil，如果等于 nil 再创建实例。
		这种方法称之为延迟初始化(Lazy Initialization)，一个对象的延迟初始化意味着该对象的创建将会延迟至第一次使用该对象时。
		主要用于提高性能，并减少程序内存要求。
	.*/
	c.lru.Add(key, value, c.ttl)
}

// get 函数用于从缓存中获取数据
func (c *LRUcache) get(key string) (value ByteView, ok bool) {
	c.mu.RLock() //读锁
	defer c.mu.RUnlock()
	if c.lru == nil {
		return
	}
	if v, ok := c.lru.Get(key); ok {
		return v.(ByteView), ok
	}
	return
}

// LFUcache 同理于LRUcache
type LFUcache struct {
	mu         sync.RWMutex
	lfu        *lfu.LFUCache
	cacheBytes int64
	ttl        time.Duration
}

// add 函数用于向缓存中添加数据
func (c *LFUcache) add(key string, value ByteView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lfu == nil {
		c.lfu = lfu.New(c.cacheBytes, nil, c.ttl)
	}
	c.lfu.Add(key, value, c.ttl)
}

// get 函数用于从缓存中获取数据
func (c *LFUcache) get(key string) (value ByteView, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lfu == nil {
		return
	}
	if v, ok := c.lfu.Get(key); ok {
		return v.(ByteView), ok
	}
	return
}
