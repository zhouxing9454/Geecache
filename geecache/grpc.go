package geecache

import (
	"Geecache/geecache/consistenthash"
	pb "Geecache/geecache/geecachepb"
	"Geecache/geecache/registry"
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

const (
	defaultReplicas = 50
)

// server 模块为geecache之间提供通信能力
// 这样部署在其他机器上的cache可以通过访问server获取缓存
// 至于找哪台主机 那是一致性哈希的工作了

var (
	defaultEtcdConfig = clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	}
)

// Server 和 Group 是解耦合的 所以server要自己实现并发控制
type Server struct {
	pb.UnimplementedGroupCacheServer

	self       string     // format: ip:port
	status     bool       // true: running false: stop
	stopSignal chan error // 通知registry revoke服务
	mu         sync.Mutex
	peers      *consistenthash.Map
	clients    map[string]*Client
}

// NewServer 创建cache的svr
func NewServer(self string) (*Server, error) {
	return &Server{
		self:    self,
		peers:   consistenthash.New(defaultReplicas, nil),
		clients: map[string]*Client{},
	}, nil
}

// Get 实现Cache service的Get接口
func (s *Server) Get(ctx context.Context, in *pb.Request) (*pb.Response, error) {
	group, key := in.Group, in.Key
	resp := &pb.Response{}
	log.Printf("[Geecache_svr %s] Recv RPC Request - (%s)/(%s)", s.self, group, key)
	if key == "" {
		return resp, fmt.Errorf("key required")
	}
	g := GetGroup(group)
	if g == nil {
		return resp, fmt.Errorf("group not found")
	}
	view, err := g.Get(key)
	if err != nil {
		return resp, err
	}
	body, err := proto.Marshal(&pb.Response{Value: view.ByteSlice()})
	if err != nil {
		log.Printf("encoding response body:%v", err)
	}
	resp.Value = body
	return resp, nil
}

// Start 启动cache服务
func (s *Server) Start() error {
	s.mu.Lock()
	if s.status == true {
		s.mu.Unlock()
		return fmt.Errorf("server already started")
	}
	// -----------------启动服务----------------------
	// 1. 设置status为true 表示服务器已在运行
	// 2. 初始化stop channel,这用于通知registry stop keep alive
	// 3. 初始化tcp socket并开始监听
	// 4. 注册rpc服务至grpc 这样grpc收到request可以分发给server处理
	// 5. 将自己的服务名/Host地址注册至etcd 这样client可以通过etcd
	//    获取服务Host地址 从而进行通信。这样的好处是client只需知道服务名
	//    以及etcd的Host即可获取对应服务IP 无需写死至client代码中
	// ----------------------------------------------
	s.status = true
	s.stopSignal = make(chan error)

	port := strings.Split(s.self, ":")[1]
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterGroupCacheServer(grpcServer, s)
	//它的作用是将一个实现了特定 gRPC 服务接口的服务器对象（在这里是 s，类型为 *Server）注册到一个 gRPC 服务器（grpcServer）上，以便这个 gRPC 服务器可以处理来自客户端的请求

	// 注册服务至etcd
	go func() {
		// Register never return unless stop signal received
		err := registry.Register("geecache", s.self, s.stopSignal)
		if err != nil {
			log.Fatalf(err.Error())
		}
		// Close channel
		close(s.stopSignal)
		// Close tcp listen
		err = lis.Close()
		if err != nil {
			log.Fatalf(err.Error())
		}
		log.Printf("[%s] Revoke service and close tcp socket ok.", s.self)
	}()

	//log.Printf("[%s] register service ok\n", s.self)
	s.mu.Unlock()

	if err := grpcServer.Serve(lis); s.status && err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}

func (s *Server) Set(peersAddr ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers.Add(peersAddr...)
	for _, peerAddr := range peersAddr {
		service := fmt.Sprintf("geecache/%s", peerAddr)
		s.clients[peerAddr] = NewClient(service)
	}
}

func (s *Server) PickPeer(key string) (PeerGetter, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	peerAddr := s.peers.Get(key)
	// Pick itself
	if peerAddr == s.self {
		log.Printf("ooh! pick myself, I am %s\n", s.self)
		return nil, false
	}
	log.Printf("[cache %s] pick remote peer: %s\n", s.self, peerAddr)
	return s.clients[peerAddr], true
}

// Stop 停止server运行 如果server没有运行 这将是一个no-op
func (s *Server) Stop() {
	s.mu.Lock()
	if s.status == false {
		s.mu.Unlock()
		return
	}
	s.stopSignal <- nil // 发送停止keepalive信号
	s.status = false    // 设置server运行状态为stop
	s.clients = nil     // 清空一致性哈希信息 有助于垃圾回收
	s.peers = nil
	s.mu.Unlock()
}

// 测试Server是否实现了Picker接口
var _ PeerPicker = (*Server)(nil)

type Client struct { // client 模块实现geecache访问其他远程节点 从而获取缓存的能力
	baseURL string // 服务名称 geecache/ip:addr
}

// Get 从remote peer获取对应缓存值
func (g *Client) Get(in *pb.Request, out *pb.Response) error {
	// 创建一个etcd client
	cli, err := clientv3.New(defaultEtcdConfig)
	if err != nil {
		return err
	}
	defer cli.Close()
	// 发现服务 取得与服务的连接
	conn, err := registry.EtcdDial(cli, g.baseURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	grpcClient := pb.NewGroupCacheClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := grpcClient.Get(ctx, in)
	if err != nil {
		return fmt.Errorf("reading response body:%v", err)
	}
	if err = proto.Unmarshal(response.GetValue(), out); err != nil {
		return fmt.Errorf("decoding response body:%v", err)
	}
	return nil
}

func NewClient(service string) *Client {
	return &Client{baseURL: service}
}

// 测试Client是否实现了Fetcher接口
var _ PeerGetter = (*Client)(nil)
