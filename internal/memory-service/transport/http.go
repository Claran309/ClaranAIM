package transport

import (
	memorydao "ClaranAIM/internal/memory-service/dao"
	memorysvc "ClaranAIM/internal/memory-service/service"
	"ClaranAIM/pkg/memoryclient"
	"ClaranAIM/pkg/servicehttp"
	"net/http"
)

// NewHTTPHandler 是当前包对外暴露的函数，负责承接对应的业务流程、参数校验或适配逻辑。
func NewHTTPHandler(svc memorysvc.MemoryService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/memory/create", func(w http.ResponseWriter, r *http.Request) {
		var req memoryclient.CreateMemoryInput
		if !decode(w, r, &req) {
			return
		}
		fact, err := svc.CreateMemory(r.Context(), req)
		respond(w, map[string]interface{}{"success": true, "memory": fact}, err)
	})
	mux.HandleFunc("/internal/memory/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ViewerID int64               `json:"viewer_id"`
			Filter   memoryclient.Filter `json:"filter"`
		}
		if !decode(w, r, &req) {
			return
		}
		facts, total, err := svc.ListMemories(r.Context(), req.ViewerID, filterToDAO(req.Filter))
		respond(w, map[string]interface{}{"success": true, "memories": facts, "total": total}, err)
	})
	mux.HandleFunc("/internal/memory/recall", func(w http.ResponseWriter, r *http.Request) {
		var req memoryclient.RecallInput
		if !decode(w, r, &req) {
			return
		}
		result, err := svc.Recall(r.Context(), req)
		respond(w, map[string]interface{}{"success": true, "result": result}, err)
	})
	mux.HandleFunc("/internal/memory/update", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ViewerID int64                          `json:"viewer_id"`
			MemoryID int64                          `json:"memory_id"`
			Input    memoryclient.UpdateMemoryInput `json:"input"`
		}
		if !decode(w, r, &req) {
			return
		}
		fact, err := svc.UpdateMemory(r.Context(), req.ViewerID, req.MemoryID, req.Input)
		respond(w, map[string]interface{}{"success": true, "memory": fact}, err)
	})
	mux.HandleFunc("/internal/memory/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ViewerID int64 `json:"viewer_id"`
			MemoryID int64 `json:"memory_id"`
		}
		if !decode(w, r, &req) {
			return
		}
		respond(w, map[string]interface{}{"success": true}, svc.DeleteMemory(r.Context(), req.ViewerID, req.MemoryID))
	})
	return mux
}

// filterToDAO 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func filterToDAO(filter memoryclient.Filter) memorydao.MemoryFilter {
	return memorydao.MemoryFilter{
		BotID:           filter.BotID,
		UserID:          filter.UserID,
		OwnerUserID:     filter.OwnerUserID,
		GroupID:         filter.GroupID,
		ConversationID:  filter.ConversationID,
		SessionID:       filter.SessionID,
		Scopes:          filter.Scopes,
		Types:           filter.Types,
		IncludeDisabled: filter.IncludeDisabled,
		Limit:           filter.Limit,
		Offset:          filter.Offset,
	}
}

// decode 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func decode(w http.ResponseWriter, r *http.Request, dest interface{}) bool {
	if r.Method != http.MethodPost {
		servicehttp.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	if err := servicehttp.Decode(r, dest); err != nil {
		servicehttp.Error(w, http.StatusBadRequest, "参数错误")
		return false
	}
	return true
}

// respond 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func respond(w http.ResponseWriter, data interface{}, err error) {
	if err != nil {
		servicehttp.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	servicehttp.OK(w, data)
}
