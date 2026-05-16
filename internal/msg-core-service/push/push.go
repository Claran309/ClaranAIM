package push

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

// PushClient WebSocket推送客户端
// msg-core-service 通过此客户端向 websocket-gateway 发送HTTP请求
// websocket-gateway 收到请求后，将消息广播给目标用户的WebSocket连接
// 这实现了"后端服务 → WebSocket网关 → 用户浏览器"的实时消息推送链路
type PushClient struct {
	wsGatewayAddr string        // websocket-gateway 的HTTP地址（如 127.0.0.1:8081）
	httpClient    *http.Client  // HTTP客户端，用于发送推送请求
}

func NewPushClient(wsGatewayAddr string) *PushClient {
	return &PushClient{
		wsGatewayAddr: wsGatewayAddr,
		httpClient: &http.Client{
			Timeout: 5 * time.Second, // 推送请求超时时间5秒
		},
	}
}

// PushMessage 推送请求结构体
// 发送到 websocket-gateway 的 /push 接口的请求体
type PushMessage struct {
	TargetUserIDs []int64     `json:"target_user_ids"` // 目标用户ID列表（消息接收者）
	Data         MessageData `json:"data"`            // 消息内容
}

// MessageData 推送消息数据
// 包含消息的完整信息，用于前端展示新消息通知
type MessageData struct {
	Type           string  `json:"type"`             // 消息类型标识：new_message/message_edited/message_recalled
	ConversationID int64   `json:"conversation_id"`  // 所属会话ID
	SenderID       int64   `json:"sender_id"`        // 发送者用户ID
	Content        string  `json:"content"`          // 消息内容
	MsgType        string  `json:"msg_type"`         // 消息类型：text/image等
	MsgID          int64   `json:"msg_id"`           // 消息ID
	CreatedAt      string  `json:"created_at"`       // 发送时间
	ReplyToID      int64   `json:"reply_to_id"`      // 引用消息ID
	Status         string  `json:"status"`           // sent/recalled
	IsEdited       bool    `json:"is_edited"`        // 是否编辑过
	EditedAt       string  `json:"edited_at"`        // 编辑时间
	MentionUserIDs []int64 `json:"mention_user_ids"` // @用户列表
	MentionAll     bool    `json:"mention_all"`      // 是否 @所有人
}

// PushMessage 向 websocket-gateway 推送消息
// 流程：组装推送数据 → JSON序列化 → HTTP POST 到 /push 接口
// websocket-gateway 收到后会将消息广播给所有目标用户的WebSocket连接
func (p *PushClient) PushMessage(targetUserIDs []int64, data MessageData) error {
	pushMsg := PushMessage{
		TargetUserIDs: targetUserIDs,
		Data:         data,
	}

	// 序列化为JSON
	body, err := json.Marshal(pushMsg)
	if err != nil {
		return err
	}

	// 发送HTTP POST请求到 websocket-gateway 的 /push 接口
	url := "http://" + p.wsGatewayAddr + "/push"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
