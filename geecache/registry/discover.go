package registry

import (
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/resolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// EtcdDial 向grpc请求一个服务，通过提供一个etcd client和service name即可获得Connection
func EtcdDial(c *clientv3.Client, service string) (*grpc.ClientConn, error) {
	etcdResolver, err := resolver.NewBuilder(c) //使用etcd客户端构建了一个服务发现的构建器。
	if err != nil {                             //检查是否在创建etcd服务发现构建器时发生了错误
		return nil, err
	}
	return grpc.Dial(
		"etcd:///"+service,                                       //指定了服务的地址
		grpc.WithResolvers(etcdResolver),                         //用于服务发现的解析器
		grpc.WithTransportCredentials(insecure.NewCredentials()), //用于设置gRPC连接的传输层安全性，这里使用了不安全的连接（insecure）
		grpc.WithBlock(),                                         //用于在连接建立之前阻塞，确保连接建立成功后再继续执行后续的代码。
	)
} // 最后返回一个指向已建立连接的grpc.ClientConn类型的指针，或者在发生错误时返回一个错误
