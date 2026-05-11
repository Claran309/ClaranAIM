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

type WSHandler struct {
	Hub        *hub.Hub
	Redis      *redis.RedisClient
}

func NewWSHandler(h *hub.Hub, r *redis.RedisClient) *WSHandler {
	return &WSHandler{Hub: h, Redis: r}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "缺少认证信息", http.StatusUnauthorized)
		return
	}

	claims, err := jwt.ParseToken(token)
	if err != nil {
		http.Error(w, "无效的Token", http.StatusUnauthorized)
		return
	}

	conn, err := client.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	wsClient := client.NewWSClient(claims.UserID, h.Hub, conn)
	hubClient := &hub.Client{
		UserID: claims.UserID,
		Hub:    h.Hub,
		Send:   wsClient.Send,
	}

	h.Hub.Register(hubClient)

	if h.Redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		h.Redis.Set(ctx, "online:user:"+strconv.FormatInt(claims.UserID, 10), "1", 30*time.Second)
		cancel()
	}

	log.Printf("用户 %d ( %s ) WebSocket连接建立", claims.UserID, claims.Username)

	go wsClient.WritePump()
	go wsClient.ReadPump()
}
