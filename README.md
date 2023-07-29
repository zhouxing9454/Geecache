# Geecache
本项目是基于极客兔兔的分布式缓存GeeCache的基础上进行编写的，这是原来项目的地址：[7天用Go从零实现分布式缓存GeeCache](https://geektutu.com/post/geecache.html)。

我们主要实现的功能如下



### 1.LRU

最近最少使用，相对于仅考虑时间因素的 FIFO 和仅考虑访问频率的 LFU，LRU 算法可以认为是相对平衡的一种淘汰算法。LRU 认为，如果数据最近被访问过，那么将来被访问的概率也会更高。LRU 算法的实现非常简单，维护一个队列，如果某条记录被访问了，则移动到队尾，那么队首则是最近最少访问的数据，淘汰该条记录即可。

我们使用`LRU`作为缓存淘汰算法。主要用以下的结构

```go
type Cache struct {
	maxBytes  int64                         // 最大存储容量
	nBytes    int64                         // 已占用的容量
	ll        *list.List                    //直接使用 Go 语言标准库实现的双向链表list.List
	cache     map[string]*list.Element      //map,键是字符串，值是双向链表中对应节点的指针
	OnEvicted func(key string, value Value) //OnEvicted是某条记录被移除时的回调函数，可以为 nil
}
```





### 2.单击并发缓存

`ByteView`结构存储真的的缓存值

```go
type ByteView struct {
	b []byte 
    //b 将会存储真实的缓存值。选择 byte 类型是为了能够支持任意的数据类型的存储，例如字符串、图片等。
}
```



并发读写`Cache`

```go
type cache struct {
	mu         sync.Mutex
	lru        *lru.Cache
	cacheBytes int64 //lru的maxBytes
}

func (c *cache) add(key string, value ByteView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		c.lru = lru.New(c.cacheBytes, nil)
	}
	c.lru.Add(key, value)
}

func (c *cache) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		return
	}
	if v, ok := c.lru.Get(key); ok {
		return v.(ByteView), ok
	}
	return
}
```



当数据不存在的时候，使用回调函数，从数据源获取数据写入缓存

```go
type Getter interface {
	Get(key string) ([]byte, error)
} //定义接口 Getter 和 回调函数 Get(key string)([]byte, error)，参数是 key，返回值是 []byte。

type GetterFunc func(key string) ([]byte, error) //定义函数类型 GetterFunc，并实现 Getter 接口的 Get 方法。

func (f GetterFunc) Get(key string) ([]byte, error) { //函数类型实现某一个接口，称之为接口型函数，方便使用者在调用时既能够传入函数作为参数，也能够传入实现了该接口的结构体作为参数。
	return f(key)
}
```





### 3.HTTP服务端

我们创建一个结构体 `HTTPPool`，作为承载节点间 HTTP 通信的核心数据结构

```go
// HTTPPool implements PeerPicker for a pool of HTTP peers.
type HTTPPool struct {
	// this peer's base URL, e.g. "https://example.net:8000"
	self     string
	basePath string
}
// Log info with server name
func (p *HTTPPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

// ServeHTTP handle all http requests
func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, p.basePath) {
		panic("HTTPPool serving unexpected path: " + r.URL.Path)
	}
	p.Log("%s %s", r.Method, r.URL.Path)
	// /<basepath>/<groupname>/<key> required
	parts := strings.SplitN(r.URL.Path[len(p.basePath):], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	groupName := parts[0]
	key := parts[1]

	group := GetGroup(groupName)
	if group == nil {
		http.Error(w, "no such group: "+groupName, http.StatusNotFound)
		return
	}

	view, err := group.Get(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(view.ByteSlice())
}
```





### 4.一致性哈希

我们创建`Map` ，它是一致性哈希算法的主数据结构，包含 4 个成员变量：Hash 函数 `hash`；虚拟节点倍数 `replicas`；哈希环 `keys`；虚拟节点与真实节点的映射表 `hashMap`，键是虚拟节点的哈希值，值是真实节点的名称。

```go
type Hash func(data []byte) uint32 //定义了函数类型 Hash，采取依赖注入的方式，允许用于替换成自定义的 Hash 函数，也方便测试时替换，默认为 crc32.ChecksumIEEE 算法。

type Map struct {
	hash     Hash
	replicas int
	keys     []int
	hashMap  map[int]string
} 
```





### 5.分布式节点

```tex
使用一致性哈希选择节点        是                                    是
    |-----> 是否是远程节点 -----> HTTP 客户端访问远程节点 --> 成功？-----> 服务端返回返回值
                    |  否                                    ↓  否
                    |----------------------------> 回退到本地节点处理。
```

我们抽象出 2 个接口，PeerPicker 的 `PickPeer()` 方法用于根据传入的 key 选择相应节点 PeerGetter。接口 PeerGetter 的 `Get()` 方法用于从对应 group 查找缓存值。PeerGetter 就对应于上述流程中的 HTTP 客户端。

```go
package geecache

// PeerPicker is the interface that must be implemented to locate
// the peer that owns a specific key.
type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
}

// PeerGetter is the interface that must be implemented by a peer.
type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
}
```



创建具体的 HTTP 客户端类 `httpGetter`，实现 PeerGetter 接口

```go
type httpGetter struct {
	baseURL string
}

func (h *httpGetter) Get(group string, key string) ([]byte, error) {
	u := fmt.Sprintf(
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(group),
		url.QueryEscape(key),
	)
	res, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned: %v", res.Status)
	}

	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}

	return bytes, nil
}

var _ PeerGetter = (*httpGetter)(nil)
```



为 HTTPPool 添加节点选择的功能

```go
const (
	defaultBasePath = "/_geecache/"
	defaultReplicas = 50
)
// HTTPPool implements PeerPicker for a pool of HTTP peers.
type HTTPPool struct {
	// this peer's base URL, e.g. "https://example.net:8000"
	self        string
	basePath    string
	mu          sync.Mutex // guards peers and httpGetters
	peers       *consistenthash.Map
	httpGetters map[string]*httpGetter // keyed by e.g. "http://10.0.0.2:8008"
}
```



实现 PeerPicker 接口

```go
// Set updates the pool's list of peers.
func (p *HTTPPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistenthash.New(defaultReplicas, nil)
	p.peers.Add(peers...)
	p.httpGetters = make(map[string]*httpGetter, len(peers))
	for _, peer := range peers {
		p.httpGetters[peer] = &httpGetter{baseURL: peer + p.basePath}
	}
}

// PickPeer picks a peer according to key
func (p *HTTPPool) PickPeer(key string) (PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		p.Log("Pick peer %s", peer)
		return p.httpGetters[peer], true
	}
	return nil, false
}

var _ PeerPicker = (*HTTPPool)(nil)
```



集成到`group`

```go
// A Group is a cache namespace and associated data loaded spread over
type Group struct {
	name      string
	getter    Getter
	mainCache cache
	peers     PeerPicker
}

// RegisterPeers registers a PeerPicker for choosing remote peer
func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
}

func (g *Group) load(key string) (value ByteView, err error) {
	if g.peers != nil {
		if peer, ok := g.peers.PickPeer(key); ok {
			if value, err = g.getFromPeer(peer, key); err == nil {
				return value, nil
			}
			log.Println("[GeeCache] Failed to get from peer", err)
		}
	}

	return g.getLocally(key)
}

func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: bytes}, nil
}
```





### 6.防止被缓存击穿

`sync.Mutex`和`sync.WaitGroup`实现`singleflight`。

```go
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
}
```





### 7.Protobuf通信

```protobuf
syntax = "proto3";

package geecachepb;

message Request {
  string group = 1;
  string key = 2;
}

message Response {
  bytes value = 1;
}

service GroupCache {
  rpc Get(Request) returns (Response);
}
```

命令：`protoc --go_out=. geecache/geecachepb/geecachepb.proto`





### 结构目录

```tex
│  go.mod
│  main.go 主函数
│  run.sh
└─geecache
    │  byteview.go 缓存值的抽象与封装
    │  cache.go 并发控制
    │  geecache.go  负责与外部交互，控制缓存存储和获取的主流程
    │  geecache_test.go
    │  http.go 节点间 HTTP 通信
    │  peers.go 节点对应的客户端
    │
    ├─consistenthash
    │      consistenthash.go 一致性哈希算法
    │      consistenthash_test.go
    │
    ├─geecachepb
    │      geecachepb.pb.go
    │      geecachepb.proto protobuf通信
    │
    ├─lru
    │      lru.go lru内存淘汰算法
    │      lru_test.go
    │
    └─singleflight
            singleflight.go 防止缓存击穿
            singleflight_test.go
```





### 项目愿景

- [ ] 将 http 通信改为 rpc 通信提⾼⽹络通信效率
- [ ] 细化锁的粒度来提⾼并发性能
- [ ] 加⼊ etcd 进⾏分布式节点的监测实现节点的动态管理
- [ ] 加⼊缓存过期机制，⾃动清理超时缓存 





### 项目改进

#### 1.实现lfu算法和lru算法两种缓存淘汰策略。

先创建lfu目录，实现lfu算法和test文件。部分结构体如下：

```go
type LFUCache struct {
	maxBytes int64 // 最大存储容量
	nBytes   int64 // 已占用的容量
	// 使用一个 heap 来管理缓存项，heap 中的元素按照频率排序
	// heap 实现了一个最小堆，即堆顶元素是最小值
	heap      *entryHeap
	cache     map[string]*entry             // map，键是字符串，值是堆中对应节点的指针
	OnEvicted func(key string, value Value) // OnEvicted 是某条记录被移除时的回调函数，可以为 nil
}
```

然后cache.go文件下，创建一个接口Basecache，实现add和get方法。同时创建一个并发的LFUcache

```go
// BaseCache 是一个接口，定义了基本的缓存操作方法。它包含了两个方法：add 和 get，用于向缓存中添加数据和从缓存中获取数据。
type BaseCache interface {
	add(key string, value ByteView)
	get(key string) (value ByteView, ok bool)
}

type LFUcache struct {
	mu         sync.Mutex
	lfu        *lfu.LFUCache
	cacheBytes int64 //lru的maxBytes
}

func (c *LFUcache) add(key string, value ByteView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lfu == nil {
		c.lfu = lfu.New(c.cacheBytes, nil)
	}
	c.lfu.Add(key, value)
}

func (c *LFUcache) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lfu == nil {
		return
	}
	if v, ok := c.lfu.Get(key); ok {
		return v.(ByteView), ok
	}
	return
}
```

最后再geecache.go文件中将group结构体和newgroup方法修改为如下：

```go
type Group struct {
	name      string              //缓存组的名称。
	getter    Getter              //实现了 Getter 接口的对象（回调），从数据源用于获取缓存数据。
	mainCache BaseCache           // 主缓存，是一个 cache 类型的实例，用于存储缓存数据。——修改为BaseCache,一个缓存接口
	peers     PeerPicker          //实现了 PeerPicker 接口的对象，用于根据键选择对等节点
	loader    *singleflight.Group //确保相同的请求只被执行一次
} //负责与用户的交互，并且控制缓存值存储和获取的流程。


func NewGroup(name string, cacheBytes int64, CacheType string, getter Getter) *Group { //增加CacheType,用来选择具体缓存淘汰算法
	if getter == nil {
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:   name,
		getter: getter,
		loader: &singleflight.Group{},
	}
	switch CacheType { //根据淘汰算法，实例化mainCache
	case "lru":
		g.mainCache = &LRUcache{cacheBytes: cacheBytes}
	case "lfu":
		g.mainCache = &LFUcache{cacheBytes: cacheBytes}
	default:
		panic("Please select the correct algorithm!")
	}
	groups[name] = g
	return g
}
```

你可以在主函数中的createGroup函数测试你想要测试的算法。

```go
func createGroup() *geecache.Group {
	return geecache.NewGroup("scores", 2<<10, "lru", geecache.GetterFunc( //lru算法做测试
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] Search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
}
```





#### 2.加入热点缓存hotCache

在group结构体中加入hotCache字段

```go
type Group struct {
	name      string              //缓存组的名称。
	getter    Getter              //实现了 Getter 接口的对象（回调），从数据源用于获取缓存数据。
	mainCache BaseCache           // 主缓存，是一个 cache 类型的实例，用于存储缓存数据。——修改为BaseCache,一个缓存接口
	hotCache  BaseCache           //mainCache 用于存储本地节点作为主节点所拥有的数据，而 hotCache 则是为了存储热门数据的缓存。
	peers     PeerPicker          //实现了 PeerPicker 接口的对象，用于根据键选择对等节点
	loader    *singleflight.Group //确保相同的请求只被执行一次
} //负责与用户的交互，并且控制缓存值存储和获取的流程。
```

修改newgroup，hotCache的最大容量是mainCache的八分之一。然后修改get方法。

```go
func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
	}
	if v, ok := g.hotCache.get(key); ok {
		log.Println("[GeeCache] hit hotCache")
		return v, nil
	}
	if v, ok := g.mainCache.get(key); ok {
		log.Println("[GeeCache] hit mainCache")
		if rand.Intn(10) == 0 {
			g.populateHotCache(key, v)
		}
		return v, nil
	}
	return g.load(key)
}
```

