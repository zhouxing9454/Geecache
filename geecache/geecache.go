package geecache

import (
	pb "Geecache/geecache/geecachepb"
	"Geecache/geecache/singleflight"
	"fmt"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type Getter interface {
	Get(key string) ([]byte, error)
} //定义接口 Getter 和 回调函数 Get(key string)([]byte, error)，参数是 key，返回值是 []byte。

type GetterFunc func(key string) ([]byte, error) //定义函数类型 GetterFunc，并实现 Getter 接口的 Get 方法。

func (f GetterFunc) Get(key string) ([]byte, error) { //函数类型实现某一个接口，称之为接口型函数，方便使用者在调用时既能够传入函数作为参数，也能够传入实现了该接口的结构体作为参数。
	return f(key)
}

type Group struct {
	name      string               //缓存组的名称。
	getter    Getter               //实现了 Getter 接口的对象（回调），从数据源用于获取缓存数据。
	mainCache BaseCache            // 主缓存，是一个 BaseCache 接口的实例，用于存储本地节点作为主节点所拥有的数据。
	hotCache  BaseCache            // hotCache 则是为了存储热门数据的缓存。
	peers     PeerPicker           //实现了 PeerPicker 接口的对象，用于根据键选择相应的缓存节点
	loader    *singleflight.Group  //确保相同的请求只被执行一次
	keys      map[string]*KeyStats //根据键key获取对应key的统计信息
} //负责与用户的交互，并且控制缓存值存储和获取的流程。

type AtomicInt int64 // 封装一个原子类，用于进行原子操作，保证并发安全.

// Add 方法用于对 AtomicInt 中的值进行原子自增
func (i *AtomicInt) Add(n int64) { //原子自增
	atomic.AddInt64((*int64)(i), n)
}

// Get 方法用于获取 AtomicInt 中的值。
func (i *AtomicInt) Get() int64 {
	return atomic.LoadInt64((*int64)(i))
}

type KeyStats struct { //Key的统计信息
	firstGetTime time.Time //第一次请求的时间
	remoteCnt    AtomicInt //请求的次数（利用atomic包封装的原子类）
}

var (
	maxMinuteRemoteQPS = 10                      //最大QPS
	mu                 sync.RWMutex              //读写锁
	groups             = make(map[string]*Group) //map,根据键缓存组的名字，获取对应的缓存组
)

// NewGroup 函数传入name,acheBytes,CacheType,getter,获取缓存组Group
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
		keys:   map[string]*KeyStats{},
	}
	switch CacheType { //根据淘汰算法，实例化mainCache,hotCache
	case "lru":
		g.mainCache = &LRUcache{cacheBytes: cacheBytes, ttl: time.Second * 60}
		g.hotCache = &LRUcache{cacheBytes: cacheBytes / 8, ttl: time.Second * 60}
	case "lfu":
		g.mainCache = &LFUcache{cacheBytes: cacheBytes, ttl: time.Second * 60}
		g.hotCache = &LFUcache{cacheBytes: cacheBytes / 8, ttl: time.Second * 60}
	default:
		panic("Please select the correct algorithm!")
	}
	groups[name] = g
	return g
}

// GetGroup 根据name获取对应的Group
func GetGroup(name string) *Group {
	mu.RLock() //只读
	g := groups[name]
	mu.RUnlock()
	return g
}

// Get 函数用于获取缓存数据，获取顺序为：热点缓存、主缓存、数据源
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
		return v, nil
	}
	return g.load(key)
}

// load 方法的逻辑是首先尝试从远程节点获取数据，如果失败或者没有配置远程节点，则回退到本地获取。
func (g *Group) load(key string) (value ByteView, err error) {
	view, err := g.loader.Do(key, func() (interface{}, error) { //singleFlight原理，相同请求只执行一次
		if g.peers != nil {
			if peer, ok := g.peers.PickPeer(key); ok { //根据key选择远程节点
				if value, err = g.getFromPeer(peer, key); err == nil { //从远程节点获取数据
					return value, nil
				}
				log.Println("[GeeCache] Failed to get from peer", err)
			}
		}
		return g.getLocally(key) //从本地获取缓存数据
	})
	if err == nil {
		return view.(ByteView), nil
	}
	return
}

// getLocally 从数据源获取数据，然后将数据添加到mainCache中
func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)
	return value, nil
}

// populateCache 将数据添加到mainCache中
func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}

// populateHotCache 将数据添加到hotCache中
func (g *Group) populateHotCache(key string, value ByteView) {
	g.hotCache.add(key, value)
}

func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
} //将实现了 PeerPicker 接口的 Server 注入到 Group 中，
// 调用 RegisterPeers 函数，我们可以将实现了 PeerPicker 接口的对象注册到 Group 结构体中。
//这样，在分布式缓存系统的运行过程中，当需要根据键选择远程节点时，可以通过调用 g.peers.PickPeer(key) 来获取合适的远程节点的 PeerGetter 对象。

// getFromPeer 实现了 PeerGetter 接口的 Client 从访问远程节点，获取缓存值。
func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	req := &pb.Request{
		Group: g.name,
		Key:   key,
	}
	res := &pb.Response{}
	err := peer.Get(req, res)
	if err != nil {
		return ByteView{}, err
	}
	//远程获取cnt++
	if stat, ok := g.keys[key]; ok {
		stat.remoteCnt.Add(1)
		//计算QPS
		interval := float64(time.Now().Unix()-stat.firstGetTime.Unix()) / 60
		qps := stat.remoteCnt.Get() / int64(math.Max(1, math.Round(interval)))
		if qps >= int64(maxMinuteRemoteQPS) {
			//存入hotCache
			g.populateHotCache(key, ByteView{b: res.Value})
			//删除映射关系,节省内存
			mu.Lock()
			delete(g.keys, key)
			mu.Unlock()
		}
	} else {
		//第一次获取
		g.keys[key] = &KeyStats{
			firstGetTime: time.Now(),
			remoteCnt:    1,
		}
	}
	return ByteView{b: res.Value}, nil
}
