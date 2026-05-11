package push

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

type PushClient struct {
	wsGatewayAddr string
	httpClient    *http.Client
}

func NewPushClient(wsGatewayAddr string) *PushClient {
	return &PushClient{
		wsGatewayAddr: wsGatewayAddr,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type PushMessage struct {
	TargetUserIDs []int64    `json:"target_user_ids"`
	Data         MessageData `json:"data"`
}

type MessageData struct {
	Type           string `json:"type"`
	ConversationID int64  `json:"conversation_id"`
	SenderID       int64  `json:"sender_id"`
	Content        string `json:"content"`
	MsgType        string `json:"msg_type"`
	MsgID          int64  `json:"msg_id"`
	CreatedAt      string `json:"created_at"`
}

func (p *PushClient) PushMessage(targetUserIDs []int64, data MessageData) error {
	pushMsg := PushMessage{
		TargetUserIDs: targetUserIDs,
		Data:         data,
	}

	body, err := json.Marshal(pushMsg)
	if err != nil {
		return err
	}

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
