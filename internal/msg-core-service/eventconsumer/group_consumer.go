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
	StartGroupEventConsumerWithReliability(ctx, consumer, svc, nil)
}

// StartGroupEventConsumerWithReliability 启动带幂等/DLQ保护的群组事件消费者。
func StartGroupEventConsumerWithReliability(ctx context.Context, consumer *eventbus.KafkaConsumer, svc service.MessageService, reliability eventbus.ReliabilityStore) {
	if consumer == nil || svc == nil {
		return
	}
	handler := eventbus.NewReliableHandler(reliability, "msg-core-service", 5, svc.ApplyGroupEvent)
	go consumer.Run(ctx, handler)
}
