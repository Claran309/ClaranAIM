package main

import (
	"ClaranAIM/internal/websocket-gateway/eventconsumer"
	"ClaranAIM/internal/websocket-gateway/handler"
	"ClaranAIM/internal/websocket-gateway/hub"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/health"
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/logger"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

func main() {
	logger.InitService("websocket-gateway")

	cfg, err := config.Load("config/websocket-gateway.yaml")
	if err != nil {
		logger.Fatal("加载配置失败", "error", err)
	}

	jwt.SetSecretKey(cfg.JWT.SecretKey)

	h := hub.NewHub()
	go h.Run()

	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			logger.Warn("Redis连接失败，在线状态将仅保存在内存", "error", err)
		} else {
			health.CheckRedis(redisClient.GetInnerClient(), "websocket-gateway")
		}
	}

	wsHandler := handler.NewWSHandler(h, redisClient)

	http.Handle("/ws", wsHandler)

	if cfg.Kafka.Enabled && len(cfg.Kafka.Brokers) > 0 {
		consumer := eventbus.NewKafkaConsumer(cfg.Kafka.Brokers, events.TopicMessageEvents, "websocket-gateway")
		defer consumer.Close()
		eventconsumer.StartMessageEventConsumer(context.Background(), consumer, h)
		logger.Info("Kafka消息事件消费已启用", "topic", events.TopicMessageEvents)
	}

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
				Type           string  `json:"type"`
				ConversationID int64   `json:"conversation_id"`
				SenderID       int64   `json:"sender_id"`
				Content        string  `json:"content"`
				MsgType        string  `json:"msg_type"`
				MsgID          int64   `json:"msg_id"`
				CreatedAt      string  `json:"created_at"`
				ReplyToID      int64   `json:"reply_to_id"`
				Status         string  `json:"status"`
				IsEdited       bool    `json:"is_edited"`
				EditedAt       string  `json:"edited_at"`
				MentionUserIDs []int64 `json:"mention_user_ids"`
				MentionAll     bool    `json:"mention_all"`
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

	http.HandleFunc("/online", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ids := h.GetOnlineUserIDs()
		data, _ := json.Marshal(map[string]interface{}{"online_users": ids})
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

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

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("websocket-gateway is running"))
	})

	if redisClient != nil {
		go syncOnlineStatusToRedis(redisClient, h)
	}

	health.LogStartup(health.ServiceInfo{
		Name:    "websocket-gateway",
		Version: "1.0.0",
		Port:    cfg.Service.Address,
	})
	if err := http.ListenAndServe(cfg.Service.Address, nil); err != nil {
		logger.Fatal("服务启动失败", "error", err)
	}
}

func syncOnlineStatusToRedis(redisClient *redis.RedisClient, h *hub.Hub) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		ids := h.GetOnlineUserIDs()
		for _, uid := range ids {
			redisClient.SetWithJitter(ctx, "online:user:"+strconv.FormatInt(uid, 10), "1", 30*time.Second, 5*time.Second)
		}
		cancel()
	}
}
