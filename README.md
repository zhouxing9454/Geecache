# Geecache
本项目是基于极客兔兔的[分布式缓存GeeCache](https://geektutu.com/post/geecache.html)的基础上进行编写的。





### 结构目录

```latex
│  go.mod
│  go.sum
│  main.go	main函数,用于测试
│  README.md	MD文档
│  run.bat	windows下测试
│  run.sh	Linux下测试
│
└─geecache
    │  byteview.go	缓存值的抽象与封装
    │  cache.go	并发控制
    │  geecache.go	负责与外部交互，控制缓存存储和获取的主流程
    │  geecache_test.go 			
    │  peers.go	抽象 PeerPicker
    │  grpc.go	Server和Client的实现
    │
    ├─consistenthash
    │      consistenthash.go	一致性哈希算法
    │      consistenthash_test.go	
    │
    ├─geecachepb
    │      geecachepb.pb.go
    │      geecachepb.proto	protobuf文件
    │      geecachepb_grpc.pb.go
    │
    ├─lfu
    │      lfu.go	LFU算法
    │      lfu_test.go
    │
    ├─lru
    │      lru.go	LRU算法
    │      lru_test.go
    │
    ├─registry	
    │      discover.go	服务发现
    │      register.go	服务注册
    │
    └─singleflight
            singleflight.go	防止缓存击穿
            singleflight_test.go
```





### 项目改进

1. 实现lfu算法和lru算法两种缓存淘汰策略。
2. 加入热点缓存hotCache
3. 设置ttl和惰性删除
4. 增加了grpc进行通信
5. 使用etcd做服务注册和服务发现





[项目GeeCache面试题(个人版，可能不够全面)](https://zhouxing9454.github.io/2023/10/23/项目GeeCache面试题/)
