package registry // Package registry模块提供服务Service注册至etcd的能力

import (
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"log"
	"time"
)

var (
	//这个变量通常用于创建etcd客户端的配置，当你不需要定制化的配置时，可以直接使用 defaultEtcdConfig 这个预定义的配置。
	defaultEtcdConfig = clientv3.Config{
		Endpoints:   []string{"localhost:2379"}, // etcd服务器的地址，这里使用本地地址和默认端口
		DialTimeout: 5 * time.Second,            // 建立连接的超时时间为5秒
	}
)

// etcdAdd 在租赁模式添加一对kv至etcd
// 四个参数分别是etcd客户端，etcd租约ID，服务名称，服务地址
func etcdAdd(c *clientv3.Client, lid clientv3.LeaseID, service string, addr string) error {
	em, err := endpoints.NewManager(c, service) //创建一个用于管理 etcd 中的服务端点（endpoints）
	if err != nil {
		return err
	}
	//该方法用于将指定的服务地址（addr）添加到 etcd 中的服务端点列表中。
	//clientv3.WithLease(lid) 选项表示使用指定的租约 ID（lid）来设置键值的生命周期。
	//如果添加服务地址成功，函数会返回 nil 表示没有错误；如果发生错误，函数会返回相应的错误信息
	return em.AddEndpoint(c.Ctx(), service+"/"+addr, endpoints.Endpoint{Addr: addr}, clientv3.WithLease(lid))
}

// Register 注册一个服务至etcd,并且在服务的生命周期内保持心跳检测，确保服务的持续在线。
// 注意 Register将不会return 如果没有error的话
func Register(service string, addr string, stop chan error) error {
	// 创建一个etcd client
	cli, err := clientv3.New(defaultEtcdConfig)
	if err != nil {
		return fmt.Errorf("create etcd client failed: %v", err)
	}
	defer cli.Close()
	// 创建一个租约 配置5秒过期
	resp, err := cli.Grant(context.Background(), 5)
	if err != nil {
		return fmt.Errorf("create lease failed: %v", err)
	}
	leaseId := resp.ID //获取了该租约的 ID
	// 注册服务
	err = etcdAdd(cli, leaseId, service, addr)
	if err != nil {
		return fmt.Errorf("add etcd record failed: %v", err)
	}
	// 设置服务心跳检测,创建了一个保持租约活动的心跳通道 ch，确保租约在生命周期内保持有效。
	ch, err := cli.KeepAlive(context.Background(), leaseId)
	if err != nil {
		return fmt.Errorf("set keepalive failed: %v", err)
	}

	log.Printf("[%s] register service ok\n", addr)
	for {
		select {
		case err := <-stop:
			if err != nil {
				log.Println(err)
			}
			return err
		case <-cli.Ctx().Done():
			log.Println("service closed")
			return nil
		case _, ok := <-ch:
			// 监听租约
			if !ok {
				log.Println("keep alive channel closed")
				_, err := cli.Revoke(context.Background(), leaseId)
				return err
			}
			//log.Printf("Recv reply from service: %s/%s, ttl:%d", service, addr, resp.TTL)
		}
	}
	/*
		函数同时监听来自 stop 通道的停止信号、cli.Ctx().Done() 的服务关闭信号以及心跳通道 ch 的消息。
		如果接收到停止信号，函数会返回；
		如果服务被关闭，函数会打印日志并返回；
		如果心跳通道被关闭，函数会撤销租约，并返回相应的错误。
	*/
}
