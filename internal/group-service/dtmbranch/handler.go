// Package dtmbranch exposes HTTP branch endpoints used by DTM Saga.
package dtmbranch

import (
	"ClaranAIM/internal/group-service/service"
	"encoding/json"
	"net/http"
)

// CreateGroupPayload is the DTM branch payload for creating group metadata.
type CreateGroupPayload struct {
	GroupID   int64   `json:"group_id"`
	Name      string  `json:"name"`
	OwnerID   int64   `json:"owner_id"`
	MemberIDs []int64 `json:"member_ids"`
}

// Handler adapts DTM HTTP branch callbacks to group-service business methods.
type Handler struct {
	svc service.GroupService
}

// NewHandler creates a DTM branch handler.
func NewHandler(svc service.GroupService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers group-service DTM branch routes on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/dtm/group/create", h.CreateGroup)
	mux.HandleFunc("/dtm/group/create_compensate", h.CreateGroupCompensate)
}

// CreateGroup executes the Saga action branch that creates group rows.
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

// CreateGroupCompensate executes the Saga compensation branch for group creation.
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
