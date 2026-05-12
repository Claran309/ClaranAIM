package hub

import (
	"sync"
)

// Client Hub中的客户端引用
// 每个WebSocket连接在Hub中对应一个Client实例
// Hub通过Client的Send通道向客户端推送消息
type Client struct {
	UserID int64     // 用户ID，用于按用户分组管理连接
	Hub    *Hub      // 所属Hub实例的引用
	Send   chan []byte // 消息发送通道，Hub通过此通道向客户端推送数据
}

// Hub WebSocket连接管理中心
// 维护所有在线用户的WebSocket连接，负责连接注册/注销和消息广播
// 核心设计模式：基于channel的事件驱动模型
//   - register channel: 新连接注册
//   - unregister channel: 连接断开注销
//   - broadcast channel: 消息广播
//
// 支持同一用户多设备多连接（如手机+电脑同时在线）
// clients 结构: map[userID] -> map[*Client]bool
type Hub struct {
	clients    map[int64]map[*Client]bool // 在线用户连接表：用户ID -> 该用户的所有连接
	broadcast  chan *BroadcastMessage      // 广播消息通道，接收待推送的消息
	register   chan *Client               // 注册通道，新连接建立时发送
	unregister chan *Client               // 注销通道，连接断开时发送
	mu         sync.RWMutex               // 读写锁，保护clients并发访问
}

// BroadcastMessage 广播消息结构
// 指定目标用户ID列表和消息数据，Hub将消息推送给这些用户的所有连接
type BroadcastMessage struct {
	TargetUserIDs []int64 // 目标用户ID列表
	Data          []byte  // 消息数据（JSON格式）
}

// NewHub 创建Hub实例
// 初始化所有channel和clients map
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[int64]map[*Client]bool),
		broadcast:  make(chan *BroadcastMessage, 256), // 缓冲256条消息
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run 启动Hub的事件循环
// 必须在单独的goroutine中运行（go h.Run()）
// 持续监听三个channel的事件，分别处理：
//   - 新连接注册：将Client添加到clients表
//   - 连接注销：从clients表移除Client并关闭Send通道
//   - 消息广播：向目标用户的所有连接发送消息
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			// 新连接注册：将Client添加到对应用户的连接集合中
			h.mu.Lock()
			if h.clients[client.UserID] == nil {
				h.clients[client.UserID] = make(map[*Client]bool)
			}
			h.clients[client.UserID][client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			// 连接断开：从对应用户的连接集合中移除
			h.mu.Lock()
			if clients, ok := h.clients[client.UserID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.Send) // 关闭Send通道，通知WritePump退出
					if len(clients) == 0 {
						delete(h.clients, client.UserID) // 用户无连接时删除整个用户条目
					}
				}
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			// 消息广播：向每个目标用户的所有连接推送消息
			h.mu.RLock()
			for _, uid := range msg.TargetUserIDs {
				if clients, ok := h.clients[uid]; ok {
					for c := range clients {
						select {
						case c.Send <- msg.Data:
							// 消息成功放入发送通道
						default:
							// 发送通道已满，说明客户端处理慢或已断开
							// 关闭通道并移除该连接
							close(c.Send)
							delete(clients, c)
						}
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Register 注册客户端连接
// 线程安全：通过channel传递，由Hub的事件循环处理
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister 注销客户端连接
// 线程安全：通过channel传递，由Hub的事件循环处理
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast 广播消息给指定用户
// 线程安全：通过channel传递，由Hub的事件循环处理
func (h *Hub) Broadcast(targetUserIDs []int64, data []byte) {
	h.broadcast <- &BroadcastMessage{
		TargetUserIDs: targetUserIDs,
		Data:          data,
	}
}

// IsOnline 检查用户是否在线
// 线程安全：使用读锁保护
func (h *Hub) IsOnline(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients, ok := h.clients[userID]
	return ok && len(clients) > 0
}

// GetOnlineUserIDs 获取所有在线用户ID列表
// 用于在线状态查询和统计
func (h *Hub) GetOnlineUserIDs() []int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var ids []int64
	for uid := range h.clients {
		ids = append(ids, uid)
	}
	return ids
}
