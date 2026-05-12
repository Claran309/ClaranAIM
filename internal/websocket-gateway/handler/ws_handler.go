package handler

import (
	"ClaranAIM/internal/websocket-gateway/client"
	"ClaranAIM/internal/websocket-gateway/hub"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/jwt"
	"context"
	"log"
	"net/http"
	"strconv"
	"time"
)

// WSHandler WebSocket连接处理器
// 处理客户端的WebSocket连接请求
// 实现了 http.Handler 接口，可直接用于 http.Handle 注册路由
type WSHandler struct {
	Hub        *hub.Hub           // WebSocket连接管理中心
	Redis      *redis.RedisClient // Redis客户端（用于在线状态缓存）
}

func NewWSHandler(h *hub.Hub, r *redis.RedisClient) *WSHandler {
	return &WSHandler{Hub: h, Redis: r}
}

// ServeHTTP 处理WebSocket连接请求
// 连接地址：ws://host:port/ws?token=JWT_TOKEN
// 流程：
//   1. 从URL参数中提取JWT Token
//   2. 验证Token有效性，提取用户信息
//   3. 将HTTP连接升级为WebSocket连接
//   4. 创建WSClient并注册到Hub
//   5. 在Redis中记录用户在线状态
//   6. 启动读写goroutine处理消息
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 第1步：从URL参数获取JWT Token
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "缺少认证信息", http.StatusUnauthorized)
		return
	}

	// 第2步：验证Token并提取用户信息
	claims, err := jwt.ParseToken(token)
	if err != nil {
		http.Error(w, "无效的Token", http.StatusUnauthorized)
		return
	}

	// 第3步：将HTTP连接升级为WebSocket连接
	conn, err := client.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	// 第4步：创建WSClient和Hub Client
	wsClient := client.NewWSClient(claims.UserID, h.Hub, conn)
	hubClient := &hub.Client{
		UserID: claims.UserID,
		Hub:    h.Hub,
		Send:   wsClient.Send,
	}

	// 注册到Hub，使其能接收推送消息
	h.Hub.Register(hubClient)

	// 第5步：在Redis中记录用户在线状态（30秒过期，需定期续期）
	if h.Redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		h.Redis.Set(ctx, "online:user:"+strconv.FormatInt(claims.UserID, 10), "1", 30*time.Second)
		cancel()
	}

	log.Printf("用户 %d ( %s ) WebSocket连接建立", claims.UserID, claims.Username)

	// 第6步：启动读写goroutine
	// WritePump: 负责向客户端推送消息和发送心跳
	// ReadPump: 负责读取客户端消息和处理断开
	go wsClient.WritePump()
	go wsClient.ReadPump()
}
