package registry

import (
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/resolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// EtcdDial 向grpc请求一个服务
// 通过提供一个etcd client和service name即可获得Connection
func EtcdDial(c *clientv3.Client, service string) (*grpc.ClientConn, error) {
	etcdResolver, err := resolver.NewBuilder(c)
	if err != nil {
		return nil, err
	}
	return grpc.Dial(
		"etcd:///"+service,
		grpc.WithResolvers(etcdResolver),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
}

//用于通过gRPC与Etcd建立连接。它通过提供Etcd服务的名称，借助Etcd解析器将其解析为实际的网络地址。
//然后，使用不安全的传输凭据建立连接，并在连接建立之前进行阻塞。
//最终，函数返回一个指向已建立连接的grpc.ClientConn类型的指针，或者在发生错误时返回一个错误
