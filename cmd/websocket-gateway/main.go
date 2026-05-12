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

// websocket-gateway 启动入口
// WebSocket网关服务，负责维护客户端WebSocket连接和实时消息推送
// 是实现即时通讯"实时性"的关键组件
//
// 提供的HTTP路由：
//   - /ws        : WebSocket连接端点（前端通过此端点建立长连接）
//   - /push      : 消息推送API（后端服务调用此接口推送消息）
//   - /online    : 在线用户查询接口
//   - /is_online : 检查指定用户是否在线
//   - /health    : 健康检查接口
//
// 启动流程：加载配置 → 初始化JWT → 创建Hub → 连接Redis → 注册路由 → 启动HTTP服务
func main() {
	// 加载配置文件（config/websocket-gateway.yaml + 环境变量）
	cfg, err := config.Load("config/websocket-gateway.yaml")
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化JWT密钥（用于WebSocket连接时验证Token）
	jwt.SetSecretKey(cfg.JWT.SecretKey)

	// 创建Hub并启动事件循环（在单独的goroutine中运行）
	h := hub.NewHub()
	go h.Run()

	// 连接Redis（用于持久化在线状态）
	var redisClient *redis.RedisClient
	if cfg.Redis.Addr != "" {
		redisClient, err = redis.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Printf("Redis连接失败，在线状态将仅保存在内存: %v", err)
		} else {
			log.Println("Redis连接成功")
		}
	}

	// 创建WebSocket处理器
	wsHandler := handler.NewWSHandler(h, redisClient)

	// ========== 注册HTTP路由 ==========

	// /ws - WebSocket连接端点
	// 前端通过 ws://host:port/ws?token=JWT_TOKEN 建立WebSocket连接
	http.Handle("/ws", wsHandler)

	// /push - 消息推送API（供后端服务调用）
	// msg-core-service 发送消息后，通过此接口将消息推送给在线用户
	// 请求体格式：{"target_user_ids": [1,2], "data": {消息内容}}
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

		// 解析推送请求
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

		// 组装WebSocket消息格式 {type, data}
		wsMsg := map[string]interface{}{
			"type": pushReq.Data.Type,
			"data": pushReq.Data,
		}
		data, _ := json.Marshal(wsMsg)

		// 通过Hub广播给目标用户的所有WebSocket连接
		h.Broadcast(pushReq.TargetUserIDs, data)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"pushed":  len(pushReq.TargetUserIDs),
		})
	})

	// /online - 获取所有在线用户ID列表
	http.HandleFunc("/online", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ids := h.GetOnlineUserIDs()
		data, _ := json.Marshal(map[string]interface{}{"online_users": ids})
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	// /is_online - 检查指定用户是否在线
	// 查询参数：?user_id=123
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

	// /health - 健康检查接口
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("websocket-gateway is running"))
	})

	// 启动Redis在线状态同步协程
	// 每10秒将Hub内存中的在线用户列表同步到Redis
	// Redis中的在线状态30秒过期，需要定期续期
	if redisClient != nil {
		go syncOnlineStatusToRedis(redisClient, h)
	}

	log.Printf("websocket-gateway 启动在 %s", cfg.Service.Address)
	if err := http.ListenAndServe(cfg.Service.Address, nil); err != nil {
		log.Fatal("websocket-gateway 启动失败:", err)
	}
}

// syncOnlineStatusToRedis 定期将内存中的在线用户状态同步到Redis
// 每10秒执行一次，将Hub中所有在线用户ID写入Redis
// Redis中的key格式：online:user:{userID}，值"1"，过期时间30秒
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

// contextWithTimeout 创建带超时的context
func contextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
