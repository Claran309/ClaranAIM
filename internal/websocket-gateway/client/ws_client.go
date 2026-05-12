package client

import (
	"ClaranAIM/internal/websocket-gateway/hub"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket连接参数配置
const (
	writeWait      = 10 * time.Second  // 写操作超时时间
	pongWait       = 60 * time.Second  // 等待Pong响应的超时时间（超过此时间未收到Pong则断开）
	pingPeriod     = (pongWait * 9) / 10 // Ping发送间隔（必须小于pongWait）
	maxMessageSize = 512               // 单条消息最大字节数
)

// Upgrader WebSocket升级器
// 将HTTP连接升级为WebSocket连接
// CheckOrigin 允许所有来源（开发阶段，生产环境应限制）
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSClient WebSocket客户端连接
// 封装单个WebSocket连接的读写逻辑
// 每个WSClient运行两个goroutine：
//   - ReadPump: 从WebSocket读取客户端消息
//   - WritePump: 向WebSocket写入服务端推送的消息
type WSClient struct {
	UserID int64              // 用户ID
	Hub    *hub.Hub           // Hub引用
	Conn   *websocket.Conn    // WebSocket连接
	Send   chan []byte         // 消息发送通道（Hub通过此通道推送消息）
}

// WSMessage WebSocket消息格式
// 前后端统一的消息格式：{type: "xxx", data: {...}}
type WSMessage struct {
	Type string      `json:"type"` // 消息类型：new_message(新消息) / system(系统消息) 等
	Data interface{} `json:"data"` // 消息数据，具体内容取决于type
}

// NewWSClient 创建WebSocket客户端实例
func NewWSClient(userID int64, h *hub.Hub, conn *websocket.Conn) *WSClient {
	return &WSClient{
		UserID: userID,
		Hub:    h,
		Conn:   conn,
		Send:   make(chan []byte, 256), // 缓冲256条消息
	}
}

// ReadPump 从WebSocket连接读取客户端消息
// 在独立的goroutine中运行
// 功能：
//   - 设置读取限制和超时
//   - 处理Pong响应（心跳保活）
//   - 读取客户端发送的消息并处理
//   - 连接断开时自动从Hub注销
func (c *WSClient) ReadPump() {
	defer func() {
		// 连接断开时从Hub注销
		c.Hub.Unregister(&hub.Client{UserID: c.UserID, Hub: c.Hub, Send: c.Send})
		c.Conn.Close()
	}()

	// 设置读取限制和超时
	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))

	// Pong处理器：收到Pong后重置读取超时时间（心跳保活机制）
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			// 非正常关闭时记录日志
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket读取错误: %v", err)
			}
			break
		}

		// 解析客户端发来的消息
		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("消息解析错误: %v", err)
			continue
		}

		log.Printf("收到用户 %d 的消息: %s", c.UserID, string(message))
	}
}

// WritePump 向WebSocket连接写入服务端推送的消息
// 在独立的goroutine中运行
// 功能：
//   - 从Send通道读取Hub推送的消息并写入WebSocket
//   - 定期发送Ping帧保持连接活跃（心跳保活）
//   - 批量发送队列中的消息，提高性能
//   - Send通道关闭时发送Close帧并退出
func (c *WSClient) WritePump() {
	// 心跳定时器
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Send通道被关闭，通知客户端连接即将关闭
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 获取消息写入器
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 批量发送：将Send通道中排队的消息一起写入，减少网络IO次数
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			// 定时发送Ping帧，保持连接活跃
			// 如果客户端在pongWait时间内未回复Pong，ReadPump会断开连接
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
