package singleflight

import "sync"

type call struct { //call 代表正在进行中，或已经结束的请求。使用 sync.WaitGroup 锁避免重入。
	wg  sync.WaitGroup
	val interface{}
	err error
}

type Group struct { //Group 是 singleflight 的主数据结构，管理不同 key 的请求(call).
	mu sync.Mutex
	m  map[string]*call
}

func (g *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()         // 如果请求正在进行中，则等待
		return c.val, c.err // 请求结束，返回结果
	}
	c := new(call)
	c.wg.Add(1)  // 发起请求前加锁
	g.m[key] = c // 添加到 g.m，表明 key 已经有对应的请求在处理
	g.mu.Unlock()

	c.val, c.err = fn() // 调用 fn，发起请求
	c.wg.Done()         // 请求结束

	g.mu.Lock()
	delete(g.m, key) // 更新 g.m
	g.mu.Unlock()

	return c.val, c.err // 返回结果
} //实现了singleFlight原理：在多个并发请求触发的回调操作里，只有第⼀个回调方法被执行，
// 其余请求（落在第⼀个回调方法执行的时间窗口里）阻塞等待第⼀个回调函数执行完成后直接取结果，
//以此保证同⼀时刻只有⼀个回调方法执行，达到防止缓存击穿的目的。
