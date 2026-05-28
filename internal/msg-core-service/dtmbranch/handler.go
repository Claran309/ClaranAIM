// Package dtmbranch 暴露 DTM Saga 使用的 HTTP 分支接口。
package dtmbranch

import (
	"ClaranAIM/internal/msg-core-service/service"
	"encoding/json"
	"net/http"
)

// CreateGroupConversationPayload 是创建群会话分支的 DTM payload。
// 网关会预分配 conversation_id 和 group_id，确保 group-service 与 msg-core-service 使用同一组业务 ID。
type CreateGroupConversationPayload struct {
	ConversationID int64   `json:"conversation_id"`
	GroupID        int64   `json:"group_id"`
	ParticipantIDs []int64 `json:"participant_ids"`
}

// Handler 将 DTM HTTP 分支回调适配为 msg-core-service 业务方法。
type Handler struct {
	svc service.MessageService
}

// NewHandler 创建 DTM 分支 handler。
func NewHandler(svc service.MessageService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes 将 msg-core-service 的 DTM 分支路由注册到 mux。
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/dtm/message/group-conversation/create", h.CreateGroupConversation)
	mux.HandleFunc("/dtm/message/group-conversation/create_compensate", h.CreateGroupConversationCompensate)
}

// CreateGroupConversation 执行 Saga 正向分支：创建群聊对应的会话记录。
func (h *Handler) CreateGroupConversation(w http.ResponseWriter, r *http.Request) {
	var payload CreateGroupConversationPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := h.svc.CreateGroupConversationWithID(r.Context(), payload.ConversationID, payload.GroupID, payload.ParticipantIDs); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// CreateGroupConversationCompensate 执行 Saga 补偿分支：撤销已创建的群会话。
func (h *Handler) CreateGroupConversationCompensate(w http.ResponseWriter, r *http.Request) {
	var payload CreateGroupConversationPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.svc.CompensateGroupConversation(r.Context(), payload.GroupID, payload.ConversationID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}
