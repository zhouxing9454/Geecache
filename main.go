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
	log.Println("geecache is running at", apiAddr)
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
