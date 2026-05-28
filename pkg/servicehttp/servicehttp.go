package servicehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Response 是内部 HTTP 微服务统一响应格式。
type Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// OK 写出 code=0 的成功 JSON 响应。
func OK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    data,
	})
}

// Error 写出统一错误 JSON 响应，HTTP 状态码和业务 code 保持一致。
func Error(w http.ResponseWriter, status int, msg string) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, map[string]interface{}{
		"code":    status,
		"message": msg,
		"data":    nil,
	})
}

// Decode 使用 json.Number 解码请求体，避免大整数 ID 精度丢失。
func Decode(r *http.Request, dest interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	return decoder.Decode(dest)
}

// Post 调用内部 HTTP 服务并解析统一 Response。
func Post(ctx context.Context, client *http.Client, baseURL, path string, reqBody interface{}, out interface{}) error {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(baseURL, path), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var decoded Response
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || decoded.Code != 0 {
		if decoded.Message != "" {
			return errors.New(decoded.Message)
		}
		return fmt.Errorf("service http request failed: HTTP %d", resp.StatusCode)
	}
	if out == nil || len(decoded.Data) == 0 || string(decoded.Data) == "null" {
		return nil
	}
	return json.Unmarshal(decoded.Data, out)
}

// writeJSON 写出带 UTF-8 Content-Type 的 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// joinURL 安全拼接 baseURL 和相对路径，避免多余或缺失斜杠。
func joinURL(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}
