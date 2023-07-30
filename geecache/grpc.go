package geecache

import (
	"Geecache/geecache/consistenthash"
	pb "Geecache/geecache/geecachepb"
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"sync"
)

type grpcGetter struct {
	baseURL string //表示将要访问的远程节点的地址
}

func (g *grpcGetter) Get(in *pb.Request, out *pb.Response) error {
	c, err := grpc.Dial(g.baseURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer c.Close()
	client := pb.NewGroupCacheClient(c)
	response, err := client.Get(context.Background(), in)
	if err != nil {
		return fmt.Errorf("reading response body:%v", err)
	}
	if err = proto.Unmarshal(response.Value, out); err != nil {
		return fmt.Errorf("decoding response body:%v", err)
	}
	return nil
}

var _ PeerGetter = (*grpcGetter)(nil)

type GRCPOOL struct {
	pb.UnimplementedGroupCacheServer
	self string

	mu          sync.Mutex
	peers       *consistenthash.Map
	grpcGetters map[string]*grpcGetter
}

func NewGrpcPool(self string) *GRCPOOL {
	return &GRCPOOL{
		self:        self,
		peers:       consistenthash.New(defaultReplicas, nil),
		grpcGetters: map[string]*grpcGetter{},
	}
}

func (p *GRCPOOL) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

func (p *GRCPOOL) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers.Add(peers...)
	for _, peer := range peers {
		p.grpcGetters[peer] = &grpcGetter{
			baseURL: peer,
		}
	}
}

func (p *GRCPOOL) PickPeer(key string) (PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		p.Log("Pick peer %s", peer)
		return p.grpcGetters[peer], true
	}
	return nil, false
}

var _ PeerPicker = (*GRCPOOL)(nil)

func (p *GRCPOOL) Get(ctx context.Context, in *pb.Request) (*pb.Response, error) {
	p.Log("%s %s", in.Group, in.Key)
	response := &pb.Response{}
	group := GetGroup(in.Group)
	if group == nil {
		p.Log("no such group %v", in.Group)
		return response, fmt.Errorf("no such group %v", in.Group)
	}
	value, err := group.Get(in.Key)
	if err != nil {
		p.Log("get key %v error %v", in.Key, err)
		return response, err
	}
	body, err := proto.Marshal(&pb.Response{Value: value.ByteSlice()})
	if err != nil {
		p.Log("encoding response body:%v", err)
	}
	response.Value = body
	return response, nil
}

func (p *GRCPOOL) Run() {
	lis, err := net.Listen("tcp", p.self)
	if err != nil {
		panic(err)
	}

	server := grpc.NewServer()
	pb.RegisterGroupCacheServer(server, p)

	reflection.Register(server)
	err = server.Serve(lis)
	if err != nil {
		panic(err)
	}
}
