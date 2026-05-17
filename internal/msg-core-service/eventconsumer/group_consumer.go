package eventconsumer

import (
	"ClaranAIM/internal/msg-core-service/service"
	"ClaranAIM/pkg/eventbus"
	"context"
)

// StartGroupEventConsumer 启动群组事件消费者。
//
// group-service 通过 Kafka 发布建群、解散、成员变更等事件；msg-core-service
// 消费这些事件后维护会话和参与者关系，使群业务不再由 api-gateway 跨服务拼装。
func StartGroupEventConsumer(ctx context.Context, consumer *eventbus.KafkaConsumer, svc service.MessageService) {
	if consumer == nil || svc == nil {
		return
	}
	go consumer.Run(ctx, svc.ApplyGroupEvent)
}
