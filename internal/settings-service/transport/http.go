package transport

import (
	settingssvc "ClaranAIM/internal/settings-service/service"
	"ClaranAIM/pkg/servicehttp"
	"ClaranAIM/pkg/settingsclient"
	"net/http"
)

// NewHTTPHandler 是当前包对外暴露的函数，负责承接对应的业务流程、参数校验或适配逻辑。
func NewHTTPHandler(svc settingssvc.SettingsService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/settings/llm-profiles/save", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID int64                              `json:"user_id"`
			Input  settingsclient.SaveLLMProfileInput `json:"input"`
		}
		if !decode(w, r, &req) {
			return
		}
		profile, err := svc.SaveLLMProfile(r.Context(), req.UserID, req.Input)
		respond(w, map[string]interface{}{"success": true, "profile": profile}, err)
	})
	mux.HandleFunc("/internal/settings/llm-profiles/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID    int64  `json:"user_id"`
			UsageType string `json:"usage_type"`
		}
		if !decode(w, r, &req) {
			return
		}
		profiles, err := svc.ListLLMProfiles(r.Context(), req.UserID, req.UsageType)
		respond(w, map[string]interface{}{"success": true, "profiles": profiles}, err)
	})
	mux.HandleFunc("/internal/settings/llm-profiles/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID    int64 `json:"user_id"`
			ProfileID int64 `json:"profile_id"`
		}
		if !decode(w, r, &req) {
			return
		}
		respond(w, map[string]interface{}{"success": true}, svc.DeleteLLMProfile(r.Context(), req.UserID, req.ProfileID))
	})
	mux.HandleFunc("/internal/settings/prompts/save", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID int64                          `json:"user_id"`
			Input  settingsclient.SavePromptInput `json:"input"`
		}
		if !decode(w, r, &req) {
			return
		}
		prompt, err := svc.SavePrompt(r.Context(), req.UserID, req.Input)
		respond(w, map[string]interface{}{"success": true, "prompt": prompt}, err)
	})
	mux.HandleFunc("/internal/settings/prompts/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID int64 `json:"user_id"`
		}
		if !decode(w, r, &req) {
			return
		}
		prompts, err := svc.ListPrompts(r.Context(), req.UserID)
		respond(w, map[string]interface{}{"success": true, "prompts": prompts}, err)
	})
	mux.HandleFunc("/internal/settings/translation/resolve", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID int64 `json:"user_id"`
		}
		if !decode(w, r, &req) {
			return
		}
		cfg, err := svc.ResolveTranslationConfig(r.Context(), req.UserID)
		respond(w, map[string]interface{}{"success": true, "config": cfg}, err)
	})
	mux.HandleFunc("/internal/settings/llm-profiles/resolve", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID    int64 `json:"user_id"`
			ProfileID int64 `json:"profile_id"`
		}
		if !decode(w, r, &req) {
			return
		}
		cfg, err := svc.ResolveLLMProfile(r.Context(), req.UserID, req.ProfileID)
		respond(w, map[string]interface{}{"success": true, "config": cfg}, err)
	})
	return mux
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
