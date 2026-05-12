package response

import (
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
)

// Response 统一API响应格式
// 所有HTTP接口返回此格式的JSON数据
// Code=0 表示成功，非0表示各类错误
type Response struct {
	Code    int         `json:"code"`    // 状态码：0=成功, -1=服务器错误, 400=请求错误, 401=未授权, 403=禁止, 404=未找到
	Message string      `json:"message"` // 响应消息
	Data    interface{} `json:"data"`    // 响应数据（成功时为业务数据，失败时为nil）
}

// Success 成功响应
// Code=0, HTTP 200
func Success(ctx *app.RequestContext, data interface{}) {
	ctx.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// Error 服务器内部错误响应
// Code=-1, HTTP 500
func Error(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusInternalServerError, Response{
		Code:    -1,
		Message: msg,
		Data:    nil,
	})
}

// BadRequest 请求参数错误响应
// Code=400, HTTP 400
func BadRequest(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusBadRequest, Response{
		Code:    400,
		Message: msg,
		Data:    nil,
	})
}

// Unauthorized 未授权响应
// Code=401, HTTP 401（Token无效或过期）
func Unauthorized(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusUnauthorized, Response{
		Code:    401,
		Message: msg,
		Data:    nil,
	})
}

// Forbidden 禁止访问响应
// Code=403, HTTP 403（无权限）
func Forbidden(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusForbidden, Response{
		Code:    403,
		Message: msg,
		Data:    nil,
	})
}

// NotFound 资源未找到响应
// Code=404, HTTP 404
func NotFound(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusNotFound, Response{
		Code:    404,
		Message: msg,
		Data:    nil,
	})
}
