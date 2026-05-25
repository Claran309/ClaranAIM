// Package eventconsumer contains bot-manager-service Kafka consumers.
package eventconsumer

import (
	"ClaranAIM/internal/bot-manager-service/dao"
	"ClaranAIM/internal/bot-manager-service/model"
	"ClaranAIM/internal/bot-manager-service/service"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/events"
	"context"
	"fmt"
	"log"
	"strings"
)

// StartAgentMentionConsumer starts the @Agent dispatcher over message.created events.
func StartAgentMentionConsumer(ctx context.Context, consumer *eventbus.KafkaConsumer, botService service.BotService, dispatchRepo dao.AgentDispatchRepository, messageClient messageservice.Client) {
	if consumer == nil || botService == nil || dispatchRepo == nil || messageClient == nil {
		return
	}
	go consumer.Run(ctx, func(ctx context.Context, envelope events.Envelope) error {
		return handleAgentMentionEvent(ctx, envelope, botService, dispatchRepo, messageClient)
	})
}

func handleAgentMentionEvent(ctx context.Context, envelope events.Envelope, botService service.BotService, dispatchRepo dao.AgentDispatchRepository, messageClient messageservice.Client) error {
	if envelope.Type != events.EventTypeMessageCreated {
		return nil
	}
	payload, err := events.DecodePayload[events.MessagePayload](envelope)
	if err != nil {
		return err
	}
	if len(payload.MentionUserIDs) == 0 || strings.TrimSpace(payload.Content) == "" {
		return nil
	}
	for _, mentionedUserID := range payload.MentionUserIDs {
		bot, err := botService.GetBotByAgentUserID(ctx, mentionedUserID)
		if err != nil {
			return err
		}
		if bot == nil || !bot.IsActive || bot.AgentUserID == payload.SenderID {
			continue
		}
		shouldRun, err := dispatchRepo.Start(ctx, &model.AgentDispatchRecord{
			EventID:        envelope.EventID,
			AgentUserID:    bot.AgentUserID,
			BotID:          bot.ID,
			SourceMsgID:    payload.MsgID,
			ConversationID: payload.ConversationID,
			SenderID:       payload.SenderID,
			Status:         "started",
		})
		if err != nil {
			return err
		}
		if !shouldRun {
			continue
		}
		result, err := botService.ChatWithBot(ctx, bot.ID, payload.SenderID, payload.ConversationID, payload.Content)
		if err != nil {
			_ = dispatchRepo.MarkFailed(ctx, envelope.EventID, bot.AgentUserID, err.Error())
			log.Printf("Agent @响应失败 bot_id=%d msg_id=%d err=%v", bot.ID, payload.MsgID, err)
			if isPermanentAgentDispatchError(err) {
				continue
			}
			return err
		}
		reply := ""
		if result != nil {
			reply = result.Reply
		}
		clientMsgID := fmt.Sprintf("agent:%s:%d", envelope.EventID, bot.AgentUserID)
		resp, err := messageClient.SendMessage(ctx, &message.SendMessageReq{
			ConversationId: payload.ConversationID,
			SenderId:       bot.AgentUserID,
			Content:        reply,
			MsgType:        "text",
			ReplyToId:      payload.MsgID,
			ClientMsgId:    clientMsgID,
		})
		if err != nil {
			_ = dispatchRepo.MarkFailed(ctx, envelope.EventID, bot.AgentUserID, err.Error())
			return err
		}
		if resp == nil || !resp.Success {
			msg := "msg-core-service返回空响应"
			if resp != nil && resp.GetMsg() != "" {
				msg = resp.GetMsg()
			}
			err := fmt.Errorf("Agent回复写入失败: %s", msg)
			_ = dispatchRepo.MarkFailed(ctx, envelope.EventID, bot.AgentUserID, err.Error())
			return err
		}
		if err := dispatchRepo.MarkCompleted(ctx, envelope.EventID, bot.AgentUserID, resp.MsgId); err != nil {
			return err
		}
	}
	return nil
}

func isPermanentAgentDispatchError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	permanentHints := []string{
		"无权操作该Agent",
		"Agent权限不足",
		"bot不存在",
		"bot已停用",
		"bot未配置API Key",
		"bot未配置Base URL",
	}
	for _, hint := range permanentHints {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}
