package client

import (
	"microservicesDemo/L7-ELK/kitex_gen/post/postservice"
	"microservicesDemo/L7-ELK/kitex_gen/user/userservice"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/circuitbreak"
	"github.com/cloudwego/kitex/pkg/loadbalance"
	etcd "github.com/kitex-contrib/registry-etcd"
	"go.uber.org/zap"
)

type Clients struct {
	UserClient userservice.Client
	PostClient postservice.Client
}

func InitClients() Clients {
	// 创建etcd解析器
	r, err := etcd.NewEtcdResolver([]string{"http://127.0.0.1:2379"})
	if err != nil {
		panic(err)
	}
	zap.L().Info("√ 创建etcd解析器成功")

	// 创建熔断器
	cbs := circuitbreak.NewCBSuite(circuitbreak.RPCInfo2Key)
	zap.L().Info("√ 创建熔断器成功")

	userClient, err := userservice.NewClient("user-service",
		client.WithResolver(r),
		client.WithRPCTimeout(5*time.Second),
		client.WithLoadBalancer(loadbalance.NewWeightedRoundRobinBalancer()),
		client.WithCircuitBreaker(cbs),
	)
	if err != nil {
		panic(err)
	}
	zap.L().Info("√ 创建user-service客户端成功")

	postClient, err := postservice.NewClient("post-service", client.WithResolver(r), client.WithRPCTimeout(5*time.Second))
	if err != nil {
		panic(err)
	}
	zap.L().Info("√ 创建post-service客户端成功")

	return Clients{
		UserClient: userClient,
		PostClient: postClient,
	}
}
