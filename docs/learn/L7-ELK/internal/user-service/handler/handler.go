package handler

import (
	"context"
	"microservicesDemo/L7-ELK/internal/user-service/service"
	"microservicesDemo/L7-ELK/kitex_gen/user"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var (
	// Metrics指标
	requestCounter  metric.Int64Counter
	requestDuration metric.Float64Histogram
	errorCounter    metric.Int64Counter
)

func initMetrics() {
	// 从全局获取 meter
	meter := otel.Meter("user-service.handler")

	var err error

	requestCounter, err = meter.Int64Counter(
		"claran_user_request_total",
		metric.WithDescription("总请求数"),
	)
	if err != nil {
		zap.L().Fatal("× 创建requestCounter失败",
			zap.Error(err),
		)
	}

	requestDuration, err = meter.Float64Histogram(
		"claran_user_request_duration_seconds",
		metric.WithDescription("请求耗时"),
		metric.WithUnit("s"),
	)
	if err != nil {
		zap.L().Fatal("× 创建requestDuration失败",
			zap.Error(err),
		)
	}

	errorCounter, err = meter.Int64Counter(
		"claran_user_errors_total",
		metric.WithDescription("总错误数"),
	)
	if err != nil {
		zap.L().Fatal("× 创建errorCounter失败",
			zap.Error(err),
		)
	}
}

// UserServiceImpl implements the last service interface defined in the IDL.
type UserServiceImpl struct {
	UserService service.UserService
}

func NewUserServiceImpl(userService service.UserService) *UserServiceImpl {
	initMetrics()
	zap.L().Info("√ 初始化UserService处理器成功")
	return &UserServiceImpl{UserService: userService}
}

// Register implements the UserServiceImpl interface.
func (s *UserServiceImpl) Register(ctx context.Context, req *user.RegisterReq) (resp *user.RegisterResp, err error) {
	startTime := time.Now()
	zap.L().Info("开始处理Register请求",
		zap.String("username", req.Username),
		zap.String("email", req.Email),
	)

	// 记录指标
	defer func() {
		duration := time.Since(startTime).Seconds()
		status := getstatus(err)

		requestDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("method", "Register"),
				attribute.String("status", status),
			))

		requestCounter.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("method", "Register"),
				attribute.String("status", status),
			))

		if err != nil {
			errorCounter.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("method", "Register"),
					attribute.String("status", status),
				))
		}

		zap.L().Info("Register请求处理完成",
			zap.Duration("duration", time.Since(startTime)),
			zap.String("status", status),
			zap.String("username", req.Username),
		)
	}()

	// 创建span - 从全局获取 tracer
	tracer := otel.Tracer("user-service")
	spanCtx, span := tracer.Start(ctx, "handler.Register",
		trace.WithAttributes(
			attribute.String("username", req.Username),
		))
	defer span.End()

	//sCtx := trace.SpanContextFromContext(ctx)
	//log.Printf("Handler -> Service, TraceID: %v", sCtx.TraceID())
	newUser, err := s.UserService.Register(spanCtx, req.Username, req.Email, req.Password)
	if err != nil {
		zap.L().Error("× 执行注册服务失败",
			zap.Error(err),
			zap.String("handler", "Register"),
			zap.String("username", req.Username),
			zap.String("email", req.Email),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "执行注册服务失败："+err.Error())
		span.SetAttributes(attribute.Bool("service.success", false))
		return resp, err
	}
	zap.L().Info("√ 注册服务执行成功",
		zap.Int64("user_id", newUser.UserID),
		zap.String("username", newUser.Username),
		zap.String("email", newUser.Email),
	)

	span.SetAttributes(
		attribute.String("username", newUser.Username),
		attribute.String("email", newUser.Email),
		attribute.Bool("service.success", true),
	)

	resp = &user.RegisterResp{
		Success: true,
		UserID:  newUser.UserID,
		Msg:     "注册成功",
	}

	return resp, nil
}

// Login implements the UserServiceImpl interface.
func (s *UserServiceImpl) Login(ctx context.Context, req *user.LoginByUsernameReq) (resp *user.LoginByUsernameResp, err error) {
	startTime := time.Now()
	zap.L().Info("开始处理Login请求",
		zap.String("username", req.Username),
	)

	// 记录指标
	defer func() {
		duration := time.Since(startTime).Seconds()
		status := getstatus(err)

		requestDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("method", "Login"),
				attribute.String("status", status),
			))

		requestCounter.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("method", "Login"),
				attribute.String("status", status),
			))

		if err != nil {
			errorCounter.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("method", "Login"),
					attribute.String("status", status),
				))
		}

		zap.L().Info("Login请求处理完成",
			zap.Duration("duration", time.Since(startTime)),
			zap.String("status", status),
			zap.String("username", req.Username),
		)
	}()

	// 创建span - 从全局获取 tracer
	tracer := otel.Tracer("user-service")
	spanCtx, span := tracer.Start(ctx, "handler.Login",
		trace.WithAttributes(
			attribute.String("username", req.Username),
		))
	defer span.End()

	newUser, err := s.UserService.Login(spanCtx, req.Username, req.Password)
	if err != nil {
		zap.L().Error("× 执行登录服务失败",
			zap.Error(err),
			zap.String("handler", "Login"),
			zap.String("username", req.Username),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "执行登录服务失败："+err.Error())
		span.SetAttributes(attribute.Bool("service.success", false))
		return resp, err
	}
	zap.L().Info("√ 登录服务执行成功",
		zap.String("username", newUser.Username),
		zap.String("email", newUser.Email),
	)

	span.SetAttributes(
		attribute.String("username", newUser.Username),
		attribute.String("email", newUser.Email),
		attribute.Bool("service.success", true),
	)

	resp = &user.LoginByUsernameResp{
		Success: true,
		Token:   newUser.Username,
		Msg:     "登录成功",
	}

	return resp, err
}

func getstatus(err error) string {
	if err == nil {
		return "success"
	}
	return "error"
}
