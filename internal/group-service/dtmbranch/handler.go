// Package dtmbranch 暴露 DTM Saga 使用的 HTTP 分支接口。
package dtmbranch

import (
	"ClaranAIM/internal/group-service/service"
	"encoding/json"
	"net/http"
)

// CreateGroupPayload 是创建群资料分支的 DTM payload。
type CreateGroupPayload struct {
	GroupID   int64   `json:"group_id"`
	Name      string  `json:"name"`
	OwnerID   int64   `json:"owner_id"`
	MemberIDs []int64 `json:"member_ids"`
}

// Handler 将 DTM HTTP 分支回调适配为 group-service 业务方法。
type Handler struct {
	svc service.GroupService
}

// NewHandler 创建 DTM 分支 handler。
func NewHandler(svc service.GroupService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes 将 group-service 的 DTM 分支路由注册到 mux。
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/dtm/group/create", h.CreateGroup)
	mux.HandleFunc("/dtm/group/create_compensate", h.CreateGroupCompensate)
}

// CreateGroup 执行 Saga 正向分支：创建群资料和成员关系。
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var payload CreateGroupPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := h.svc.CreateGroupWithID(r.Context(), payload.GroupID, payload.Name, payload.OwnerID, payload.MemberIDs); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// CreateGroupCompensate 执行 Saga 补偿分支：删除创建群分支写入的数据。
func (h *Handler) CreateGroupCompensate(w http.ResponseWriter, r *http.Request) {
	var payload CreateGroupPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteGroup(r.Context(), payload.GroupID, payload.OwnerID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}
