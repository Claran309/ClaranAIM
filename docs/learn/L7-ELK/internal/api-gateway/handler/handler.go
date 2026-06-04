package handler

import (
	"context"
	client2 "microservicesDemo/L7-ELK/internal/api-gateway/client"
	"microservicesDemo/L7-ELK/kitex_gen/post"
	"microservicesDemo/L7-ELK/kitex_gen/user"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

var (
	requestCounter  metric.Int64Counter
	requestDuration metric.Float64Histogram
)

func initMetrics() {
	meter := otel.GetMeterProvider().Meter("api-gateway")

	var err error
	requestCounter, err = meter.Int64Counter(
		"claran_api_gateway_requests_total",
		metric.WithDescription("Total number of API requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		hlog.Errorf("Failed to create request counter: %v", err)
	}

	requestDuration, err = meter.Float64Histogram(
		"claran_api_gateway_request_duration_seconds",
		metric.WithDescription("API request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		hlog.Errorf("Failed to create request duration histogram: %v", err)
	}

	zap.L().Info("√ 创建指标成功：requests_total，request_duration_seconds")
}

type HttpHandler struct {
	client client2.Clients
}

func NewHttpHandler(client client2.Clients) *HttpHandler {
	initMetrics()

	return &HttpHandler{
		client: client,
	}
}

func (h *HttpHandler) Register(c context.Context, ctx *app.RequestContext) {
	startTime := time.Now()

	// 获取当前Trace上下文
	tracer := otel.Tracer("api-gateway-handler")
	spanCtx, span := tracer.Start(c, "register-handler")
	defer span.End()
	zap.L().Info("√ 获取Trace成功：handler")

	// 设置Span属性
	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("http.route", "/api/v1/users/register"),
		attribute.String("user.agent", string(ctx.UserAgent())),
	)

	// 记录自定义事件
	span.AddEvent("starting_user_registration")

	var req struct {
		Name     string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	if err := ctx.BindAndValidate(&req); err != nil {
		zap.L().Error("× 绑定参数时发生错误：",
			zap.Error(err),
			zap.String("handler", "register"),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		ctx.JSON(400, utils.H{
			"code": 400,
			"msg":  "绑定参数时发生错误",
		})
		return
	}

	rpcReq := user.RegisterReq{
		Username: req.Name,
		Password: req.Password,
		Email:    req.Email,
	}

	rpcResp, err := h.client.UserClient.Register(c, &rpcReq)
	if err != nil {
		zap.L().Error("× RPC调用失败: ：",
			zap.Error(err),
			zap.String("handler", "login"),
			zap.String("rpc_method", "UserClient.Login"),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		ctx.JSON(500, utils.H{
			"code":    500,
			"message": "服务内部错误: " + err.Error(),
		})
		return
	}
	zap.L().Info("× RPC调用成功: user-service.Register")

	statusCode := 200
	if !rpcResp.Success {
		statusCode = 400
	}
	duration := time.Since(startTime)
	durationSeconds := time.Since(startTime).Seconds()

	requestCounter.Add(spanCtx, 1,
		metric.WithAttributes(
			attribute.String("endpoint", "/register"),
			attribute.String("method", "POST"),
			attribute.String("status", "success"),
		))
	requestDuration.Record(spanCtx, durationSeconds,
		metric.WithAttributes(
			attribute.String("endpoint", "/register"),
		))

	// 返回HTTP响应
	zap.L().Info("用户注册成功",
		zap.Int64("user_id", rpcResp.UserID),
		zap.Bool("success", rpcResp.Success),
		zap.Float64("duration", duration.Seconds()),
		zap.String("endpoint", "/register"),
		zap.String("method", "POST"),
	)

	ctx.JSON(statusCode, utils.H{
		"code":    statusCode,      // HTTP状态码
		"success": rpcResp.Success, // 业务是否成功
		"message": rpcResp.Msg,     // 业务消息
	})
}

func (h *HttpHandler) Login(c context.Context, ctx *app.RequestContext) {
	startTime := time.Now()

	// 获取当前Trace上下文
	tracer := otel.Tracer("api-gateway-handler")
	spanCtx, span := tracer.Start(c, "login-handler")
	defer span.End()
	zap.L().Info("√ 获取Trace成功：login-handler")

	// 设置Span属性
	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("http.route", "/api/v1/users/login"),
		attribute.String("user.agent", string(ctx.UserAgent())),
	)

	// 记录自定义事件
	span.AddEvent("starting_user_login")

	var req struct {
		Name     string `json:"username"`
		Password string `json:"password"`
	}

	if err := ctx.BindAndValidate(&req); err != nil {
		zap.L().Error("× 绑定参数时发生错误",
			zap.Error(err),
			zap.String("handler", "login"),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		ctx.JSON(400, utils.H{
			"code": 400,
			"msg":  "绑定参数时发生错误",
		})
		return
	}

	rpcReq := user.LoginByUsernameReq{
		Username: req.Name,
		Password: req.Password,
	}

	rpcResp, err := h.client.UserClient.Login(c, &rpcReq)
	if err != nil {
		zap.L().Error("× RPC调用失败",
			zap.Error(err),
			zap.String("handler", "login"),
			zap.String("rpc_method", "UserClient.Login"),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		ctx.JSON(500, utils.H{
			"code":    500,
			"message": "服务内部错误: " + err.Error(),
		})
		return
	}
	zap.L().Info("√ RPC调用成功: user-service.Login")

	statusCode := 200
	if !rpcResp.Success {
		statusCode = 400
	}
	duration := time.Since(startTime)
	durationSeconds := time.Since(startTime).Seconds()

	requestCounter.Add(spanCtx, 1,
		metric.WithAttributes(
			attribute.String("endpoint", "/login"),
			attribute.String("method", "POST"),
			attribute.String("status", "success"),
		))
	requestDuration.Record(spanCtx, durationSeconds,
		metric.WithAttributes(
			attribute.String("endpoint", "/login"),
		))

	// 返回HTTP响应
	zap.L().Info("用户登录处理完成",
		zap.String("token", rpcResp.Token),
		zap.Int("status_code", statusCode),
		zap.Duration("duration", duration),
		zap.Bool("success", rpcResp.Success),
		zap.String("endpoint", "/login"),
		zap.String("method", "POST"),
	)

	ctx.JSON(statusCode, utils.H{
		"code":    statusCode,      // HTTP状态码
		"success": rpcResp.Success, // 业务是否成功
		"message": rpcResp.Msg,     // 业务消息
	})
}

func (h *HttpHandler) CreatePost(c context.Context, ctx *app.RequestContext) {
	startTime := time.Now()

	// 获取当前Trace上下文
	tracer := otel.Tracer("api-gateway-handler")
	spanCtx, span := tracer.Start(c, "create-post-handler")
	defer span.End()
	zap.L().Info("√ 获取Trace成功：create-post-handler")

	// 设置Span属性
	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("http.route", string(ctx.Request.URI().Path())),
		attribute.String("user.agent", string(ctx.UserAgent())),
	)

	// 记录自定义事件
	span.AddEvent("starting_post_creation")

	var req struct {
		UserID  int64  `json:"user_id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	if err := ctx.BindAndValidate(&req); err != nil {
		zap.L().Error("× 绑定参数时发生错误",
			zap.Error(err),
			zap.String("handler", "create-post"),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		ctx.JSON(400, utils.H{
			"code": 400,
			"msg":  "绑定参数时发生错误",
		})
		return
	}

	rpcReq := post.CreatePostReq{
		UserID:   req.UserID,
		PostName: req.Title,
		Content:  req.Content,
	}

	zap.L().Info("开始RPC调用: post-service.CreatePost",
		zap.Int64("user_id", req.UserID),
		zap.String("title", req.Title),
		zap.Int("content_length", len(req.Content)),
	)

	rpcResp, err := h.client.PostClient.CreatePost(c, &rpcReq)
	if err != nil {
		zap.L().Error("× RPC调用失败",
			zap.Error(err),
			zap.String("handler", "create-post"),
			zap.String("rpc_method", "PostClient.CreatePost"),
			zap.Int64("user_id", req.UserID),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		ctx.JSON(500, utils.H{
			"code":    500,
			"message": "服务内部错误: " + err.Error(),
		})
		return
	}
	zap.L().Info("√ RPC调用成功: post-service.CreatePost")

	statusCode := 200
	if !rpcResp.Success {
		statusCode = 400
	}
	duration := time.Since(startTime)
	durationSeconds := time.Since(startTime).Seconds()

	// 记录指标
	status := "success"
	if !rpcResp.Success {
		status = "failed"
	}

	requestCounter.Add(spanCtx, 1,
		metric.WithAttributes(
			attribute.String("endpoint", "/create-post"),
			attribute.String("method", "POST"),
			attribute.String("status", status),
		))
	requestDuration.Record(spanCtx, durationSeconds,
		metric.WithAttributes(
			attribute.String("endpoint", "/create-post"),
		))

	// 返回HTTP响应
	zap.L().Info("帖子创建处理完成",
		zap.Int("status_code", statusCode),
		zap.Duration("duration", duration),
		zap.Bool("success", rpcResp.Success),
		zap.String("endpoint", "/create-post"),
		zap.String("method", "POST"),
		zap.Int64("post_id", rpcResp.PostID),
		zap.Int64("user_id", req.UserID),
	)

	ctx.JSON(statusCode, utils.H{
		"code":    statusCode,      // HTTP状态码
		"success": rpcResp.Success, // 业务是否成功
		"message": rpcResp.Msg,     // 业务消息
		"post_id": rpcResp.PostID,  // 帖子ID
	})
}

func (h *HttpHandler) DeletePost(c context.Context, ctx *app.RequestContext) {
	startTime := time.Now()

	var req struct {
		UserID int64 `json:"user_id"`
		PostID int64 `json:"post_id"`
	}

	if err := ctx.BindAndValidate(&req); err != nil {
		zap.L().Error("× 绑定参数时发生错误",
			zap.Error(err),
			zap.String("handler", "delete-post"),
		)
		ctx.JSON(400, utils.H{
			"code": 400,
			"msg":  "绑定参数时发生错误",
		})
		return
	}

	rpcReq := post.DeletePostReq{
		UserID: req.UserID,
		PostID: req.PostID,
	}

	zap.L().Info("开始RPC调用: post-service.DeletePost",
		zap.Int64("user_id", req.UserID),
		zap.Int64("post_id", req.PostID),
	)

	rpcResp, err := h.client.PostClient.DeletePost(c, &rpcReq)
	if err != nil {
		zap.L().Error("× RPC调用失败",
			zap.Error(err),
			zap.String("handler", "delete-post"),
			zap.String("rpc_method", "PostClient.DeletePost"),
			zap.Int64("user_id", req.UserID),
			zap.Int64("post_id", req.PostID),
		)
		ctx.JSON(500, utils.H{
			"code":    500,
			"message": "服务内部错误: " + err.Error(),
		})
		return
	}
	zap.L().Info("√ RPC调用成功: post-service.DeletePost")

	statusCode := 200
	if !rpcResp.Success {
		statusCode = 400
	}
	duration := time.Since(startTime)

	// 返回HTTP响应
	zap.L().Info("帖子删除处理完成",
		zap.Int("status_code", statusCode),
		zap.Duration("duration", duration),
		zap.Bool("success", rpcResp.Success),
		zap.String("endpoint", "/delete-post"),
		zap.String("method", "POST"),
		zap.Int64("user_id", req.UserID),
		zap.Int64("post_id", req.PostID),
	)

	ctx.JSON(statusCode, utils.H{
		"code":    statusCode,      // HTTP状态码
		"success": rpcResp.Success, // 业务是否成功
		"message": rpcResp.Msg,     // 业务消息
	})
}
