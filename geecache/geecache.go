package geecache

import (
	pb "Geecache/geecache/geecachepb"
	"Geecache/geecache/singleflight"
	"fmt"
	"log"
	"math/rand"
	"sync"
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
	name      string              //缓存组的名称。
	getter    Getter              //实现了 Getter 接口的对象（回调），从数据源用于获取缓存数据。
	mainCache BaseCache           // 主缓存，是一个 cache 类型的实例，用于存储缓存数据。——修改为BaseCache,一个缓存接口
	hotCache  BaseCache           //mainCache 用于存储本地节点作为主节点所拥有的数据，而 hotCache 则是为了存储热门数据的缓存。
	peers     PeerPicker          //实现了 PeerPicker 接口的对象，用于根据键选择对等节点
	loader    *singleflight.Group //确保相同的请求只被执行一次
} //负责与用户的交互，并且控制缓存值存储和获取的流程。

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

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
		g.mainCache = &LRUcache{cacheBytes: cacheBytes, ttl: time.Second * 10}
		g.hotCache = &LRUcache{cacheBytes: cacheBytes / 8, ttl: time.Second * 10}
	case "lfu":
		g.mainCache = &LFUcache{cacheBytes: cacheBytes, ttl: time.Second * 10}
		g.hotCache = &LFUcache{cacheBytes: cacheBytes / 8, ttl: time.Second * 10}
	default:
		panic("Please select the correct algorithm!")
	}
	groups[name] = g
	return g
}

func GetGroup(name string) *Group {
	mu.RLock() //只读
	g := groups[name]
	mu.RUnlock()
	return g
}

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

func (g *Group) load(key string) (value ByteView, err error) {
	viewi, err := g.loader.Do(key, func() (interface{}, error) {
		if g.peers != nil {
			if peer, ok := g.peers.PickPeer(key); ok {
				if value, err = g.getFromPeer(peer, key); err == nil {
					return value, nil
				}
				log.Println("[GeeCache] Failed to get from peer", err)
			}
		}
		return g.getLocally(key)
	})
	if err == nil {
		return viewi.(ByteView), nil
	}
	return
} //使用 PickPeer() 方法选择节点，若非本机节点，则调用 getFromPeer() 从远程获取。若是本机节点或失败，则回退到 getLocally()。

func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)
	return value, nil
}

func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}

func (g *Group) populateHotCache(key string, value ByteView) {
	if g.hotCache != nil {
		// Add the data to hotCache
		g.hotCache.add(key, value)
	}
}

func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
} //将实现了 PeerPicker 接口的 HTTPPool 注入到 Group 中，
// 调用 RegisterPeers 函数，我们可以将实现了 PeerPicker 接口的对象注册到 Group 结构体中。
//这样，在分布式缓存系统的运行过程中，当需要根据键选择远程节点时，可以通过调用 g.peers.PickPeer(key) 来获取合适的远程节点的 PeerGetter 对象。

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
	return ByteView{b: res.Value}, nil
} //实现了 PeerGetter 接口的 httpGetter 从访问远程节点，获取缓存值。
