package response

import (
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func Success(ctx *app.RequestContext, data interface{}) {
	ctx.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func Error(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusInternalServerError, Response{
		Code:    -1,
		Message: msg,
		Data:    nil,
	})
}

func BadRequest(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusBadRequest, Response{
		Code:    400,
		Message: msg,
		Data:    nil,
	})
}

func Unauthorized(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusUnauthorized, Response{
		Code:    401,
		Message: msg,
		Data:    nil,
	})
}

func Forbidden(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusForbidden, Response{
		Code:    403,
		Message: msg,
		Data:    nil,
	})
}

func NotFound(ctx *app.RequestContext, msg string) {
	ctx.JSON(http.StatusNotFound, Response{
		Code:    404,
		Message: msg,
		Data:    nil,
	})
}
