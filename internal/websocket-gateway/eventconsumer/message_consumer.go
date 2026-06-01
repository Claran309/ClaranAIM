package eventconsumer

import (
	"ClaranAIM/internal/websocket-gateway/hub"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/events"
	"context"
	"log"
)

// StartMessageEventConsumer 启动消息事件消费者并转发到 WebSocket Hub。
//
// msg-core-service 将新消息、编辑、撤回、已读等事件发布到 Kafka；websocket-gateway
// 消费后转换为前端协议并广播给目标用户，从而解耦消息写库与实时推送。
func StartMessageEventConsumer(ctx context.Context, consumer *eventbus.KafkaConsumer, h *hub.Hub) {
	StartMessageEventConsumerWithReliability(ctx, consumer, h, nil)
}

// StartMessageEventConsumerWithReliability 启动带幂等/DLQ保护的消息事件消费者。
func StartMessageEventConsumerWithReliability(ctx context.Context, consumer *eventbus.KafkaConsumer, h *hub.Hub, reliability eventbus.ReliabilityStore) {
	if consumer == nil || h == nil {
		return
	}
	handler := eventbus.NewReliableHandler(reliability, "websocket-gateway", 5, func(ctx context.Context, envelope events.Envelope) error {
		payload, err := events.DecodePayload[events.MessagePayload](envelope)
		if err != nil {
			return err
		}
		data, err := payload.WebSocketMessage()
		if err != nil {
			return err
		}
		if len(payload.TargetUserIDs) == 0 {
			log.Printf("跳过无目标用户的消息事件 type=%s key=%s", envelope.Type, envelope.Key)
			return nil
		}
		h.Broadcast(payload.TargetUserIDs, data)
		return nil
	})
	go consumer.Run(ctx, handler)
}
