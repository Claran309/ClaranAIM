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

const agentDispatchHistoryLimit int64 = 80

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
	if strings.TrimSpace(payload.Content) == "" {
		return nil
	}
	agentUserIDs := agentTargetsFromMessage(payload)
	if len(agentUserIDs) == 0 {
		return nil
	}
	for _, agentUserID := range agentUserIDs {
		bot, err := botService.GetBotByAgentUserID(ctx, agentUserID)
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
		agentInput, err := buildAgentDispatchInput(ctx, messageClient, payload, bot.AgentUserID)
		if err != nil {
			_ = dispatchRepo.MarkFailed(ctx, envelope.EventID, bot.AgentUserID, err.Error())
			return err
		}
		result, err := botService.ChatWithBot(ctx, bot.ID, payload.SenderID, payload.ConversationID, agentInput)
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

func agentTargetsFromMessage(payload events.MessagePayload) []int64 {
	targets := make([]int64, 0)
	if payload.ConversationType == "private" {
		for _, participantID := range payload.ParticipantIDs {
			if participantID > 0 && participantID != payload.SenderID {
				targets = append(targets, participantID)
			}
		}
		return dedupePositiveIDs(targets)
	}
	return dedupePositiveIDs(payload.MentionUserIDs)
}

func buildAgentDispatchInput(ctx context.Context, messageClient messageservice.Client, payload events.MessagePayload, agentUserID int64) (string, error) {
	contextText := ""
	if messageClient != nil && payload.ConversationID > 0 && agentUserID > 0 {
		resp, err := messageClient.GetHistory(ctx, &message.GetHistoryReq{
			ConversationId: payload.ConversationID,
			UserId:         agentUserID,
			Limit:          agentDispatchHistoryLimit,
		})
		if err != nil {
			return "", fmt.Errorf("读取Agent可见会话上下文失败: %w", err)
		}
		if resp != nil && resp.Success {
			contextText = formatMessagesForAgentContext(resp.Messages)
		} else if resp != nil && resp.Msg != "" {
			return "", fmt.Errorf("读取Agent可见会话上下文失败: %s", resp.Msg)
		}
	}
	if contextText == "" {
		contextText = "（当前没有读取到历史消息。请基于用户这条消息本身回答，并说明上下文很少。）"
	}
	return fmt.Sprintf("用户在 IM 会话中触发了你，请基于真实会话材料回复，而不是只依赖你自己的长期记忆。\n\n当前用户消息：\n%s\n\n会话材料说明：下面是 msg-core-service 从当前会话读取到的、Agent 用户有权看到的历史消息，按时间从旧到新排列；它们是本轮回答的主要事实来源。\n\n会话材料：\n%s", strings.TrimSpace(payload.Content), contextText), nil
}

func formatMessagesForAgentContext(messages []*message.Message) string {
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			content = fmt.Sprintf("[%s消息]", msg.MsgType)
		}
		content = strings.ReplaceAll(content, "\r\n", "\n")
		content = strings.ReplaceAll(content, "\n", " ")
		if len([]rune(content)) > 600 {
			content = string([]rune(content)[:600]) + "..."
		}
		fmt.Fprintf(&b, "- [%s] 用户%d: %s\n", msg.CreatedAt, msg.SenderId, content)
	}
	return strings.TrimSpace(b.String())
}

func dedupePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
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
