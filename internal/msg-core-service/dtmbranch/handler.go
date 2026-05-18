// Package dtmbranch exposes HTTP branch endpoints used by DTM Saga.
package dtmbranch

import (
	"ClaranAIM/internal/msg-core-service/service"
	"encoding/json"
	"net/http"
)

// CreateGroupConversationPayload is the DTM branch payload for creating the
// msg-core-service conversation that backs a group.
type CreateGroupConversationPayload struct {
	ConversationID int64   `json:"conversation_id"`
	GroupID        int64   `json:"group_id"`
	ParticipantIDs []int64 `json:"participant_ids"`
}

// Handler adapts DTM HTTP branch callbacks to msg-core-service business methods.
type Handler struct {
	svc service.MessageService
}

// NewHandler creates a DTM branch handler.
func NewHandler(svc service.MessageService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers msg-core-service DTM branch routes on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/dtm/message/group-conversation/create", h.CreateGroupConversation)
	mux.HandleFunc("/dtm/message/group-conversation/create_compensate", h.CreateGroupConversationCompensate)
}

// CreateGroupConversation executes the Saga action branch that creates the group conversation.
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

// CreateGroupConversationCompensate executes the Saga compensation branch.
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
