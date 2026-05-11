package main

import (
	"ClaranAIM/internal/websocket-gateway/handler"
	"ClaranAIM/internal/websocket-gateway/hub"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/jwt"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

func main() {
	cfg, err := config.Load("config/websocket-gateway.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	jwt.SetSecretKey(cfg.JWT.SecretKey)

	h := hub.NewHub()
	go h.Run()

	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Printf("Redis连接失败，在线状态将仅保存在内存: %v", err)
		} else {
			log.Println("Redis连接成功")
		}
	}

	wsHandler := handler.NewWSHandler(h, redisClient)

	// WebSocket连接端点
	http.Handle("/ws", wsHandler)

	// 消息推送API（供后端服务调用）
	http.HandleFunc("/push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "读取请求体失败", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var pushReq struct {
			TargetUserIDs []int64 `json:"target_user_ids"`
			Data          struct {
				Type           string `json:"type"`
				ConversationID int64  `json:"conversation_id"`
				SenderID       int64  `json:"sender_id"`
				Content        string `json:"content"`
				MsgType        string `json:"msg_type"`
				MsgID          int64  `json:"msg_id"`
				CreatedAt      string `json:"created_at"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &pushReq); err != nil {
			http.Error(w, "解析请求失败", http.StatusBadRequest)
			return
		}

		wsMsg := map[string]interface{}{
			"type": pushReq.Data.Type,
			"data": pushReq.Data,
		}
		data, _ := json.Marshal(wsMsg)
		h.Broadcast(pushReq.TargetUserIDs, data)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"pushed":  len(pushReq.TargetUserIDs),
		})
	})

	// 在线用户查询
	http.HandleFunc("/online", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ids := h.GetOnlineUserIDs()
		data, _ := json.Marshal(map[string]interface{}{"online_users": ids})
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// 检查用户是否在线
	http.HandleFunc("/is_online", func(w http.ResponseWriter, r *http.Request) {
		userIDStr := r.URL.Query().Get("user_id")
		userID, _ := strconv.ParseInt(userIDStr, 10, 64)
		w.Header().Set("Content-Type", "application/json")
		online := h.IsOnline(userID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user_id": userID,
			"online":  online,
		})
	})

	// 健康检查
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("websocket-gateway is running"))
	})

	// Redis在线状态同步：定期将内存中的在线用户同步到Redis
	if redisClient != nil {
		go syncOnlineStatusToRedis(redisClient, h)
	}

	log.Printf("websocket-gateway 启动在 %s", cfg.Service.Address)
	if err := http.ListenAndServe(cfg.Service.Address, nil); err != nil {
		log.Fatal("websocket-gateway 启动失败:", err)
	}
}

func syncOnlineStatusToRedis(redisClient *redis.RedisClient, h *hub.Hub) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := contextWithTimeout(3 * time.Second)
		ids := h.GetOnlineUserIDs()
		for _, uid := range ids {
			redisClient.Set(ctx, "online:user:"+strconv.FormatInt(uid, 10), "1", 30*time.Second)
		}
		cancel()
	}
}

func contextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
