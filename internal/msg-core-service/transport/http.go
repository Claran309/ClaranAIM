package transport

import (
	msgsvc "ClaranAIM/internal/msg-core-service/service"
	"ClaranAIM/pkg/messageclient"
	"ClaranAIM/pkg/servicehttp"
	"net/http"
)

// NewHTTPHandler 是当前包对外暴露的函数，负责承接对应的业务流程、参数校验或适配逻辑。
func NewHTTPHandler(svc msgsvc.MessageService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/message/translate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			servicehttp.Error(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req messageclient.TranslateMessageInput
		if err := servicehttp.Decode(r, &req); err != nil {
			servicehttp.Error(w, http.StatusBadRequest, "参数错误")
			return
		}
		result, err := svc.TranslateMessage(r.Context(), msgsvc.TranslateMessageInput{
			MessageID:      req.MessageID,
			UserID:         req.UserID,
			TargetLanguage: req.TargetLanguage,
			Force:          req.Force,
		})
		if err != nil {
			servicehttp.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		servicehttp.OK(w, map[string]interface{}{"success": true, "translation": result.ToClient()})
	})
	return mux
}
