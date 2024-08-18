package main

import (
	"Geecache/geecache"
	"flag"
	"fmt"
	"log"
	"net/http"
)

// db 是伪造的数据源
var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

// createGroup 创建并返回一个 geecache 的缓存组（Group 实例）。
// 该组使用 LRU 策略，并且有一个 Getter 函数，用于从 db 字典中获取数据。
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

// startAPIServer 启动一个 API 服务器，用于与用户进行交互。用户可以通过访问 /api?key=XXX 的形式来获取缓存数据。
func startAPIServer(apiAddr string, gee *geecache.Group) {
	http.Handle("/api", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("key")
			view, err := gee.Get(key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream") //二进制数据流媒体类型
			w.Write(view.ByteSlice())
		}))
	log.Println("geecache api is running at", apiAddr)
	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))
}

func main() {
	var port int
	var api bool
	flag.IntVar(&port, "port", 8001, "Geecache server port")
	flag.BoolVar(&api, "api", false, "Start a api server?")
	flag.Parse()

	apiAddr := "http://localhost:9999"
	addrMap := map[int]string{
		8001: "127.0.0.1:8001",
		8002: "127.0.0.1:8002",
		8003: "127.0.0.1:8003",
	} //grpc版本（含etcd）
	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}
	gee := createGroup()
	if api {
		go startAPIServer(apiAddr, gee)
	}
	startCacheServerGrpcEtcd(addrMap[port], addrs, gee) //grpc版本
}

// startCacheServerGrpcEtcd 函数：
// 创建一个 geecache.Server 实例，该实例用于处理 gRPC 请求并与其他节点通信。
// 通过 geecache.Server 实例的 Set 方法设置一组节点地址。
// 将 geecache.Server 实例注册到缓存组（gee）中。
// 启动 geecache.Server 实例，开始处理 gRPC 请求。
func startCacheServerGrpcEtcd(addr string, addrs []string, gee *geecache.Group) {
	peers, _ := geecache.NewServer(addr)
	peers.Set(addrs...)
	gee.RegisterPeers(peers)
	log.Println("geecache is running at ", addr)
	err := peers.Start()
	if err != nil {
		peers.Stop()
	}
}

/*
用户通过 API 服务器（例如 http://localhost:9999）访问 /api?key=XXX 的形式来获取缓存数据。
API 服务器会调用对应缓存组的 Get 方法。
Get 方法首先尝试从本地缓存中的热点缓存（hotCache）中查找数据。
如果数据不在热点缓存中，它将尝试从主缓存（mainCache）中查找数据。
如果主缓存中也没有数据，缓存系统将选择一个远程节点（可能是本地节点，也可能是其他节点）。
如果选中的远程节点是本地节点，缓存系统会直接从数据源获取数据。
如果选中的远程节点不是本地节点，API 服务器将发送 gRPC 请求给对应的远程节点，要求其提供数据。

注意：节点刚开始都会注册到etcd。
*/
