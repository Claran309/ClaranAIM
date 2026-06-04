package handler

import (
	"context"
	"microservicesDemo/L7-ELK/internal/post-service/service"
	"microservicesDemo/L7-ELK/kitex_gen/post"
	"strconv"
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
	meter := otel.Meter("post-service.handler")

	var err error

	requestCounter, err = meter.Int64Counter(
		"claran_post_request_total",
		metric.WithDescription("总请求数"),
	)
	if err != nil {
		zap.L().Fatal("× 创建requestCounter失败",
			zap.Error(err),
		)
	}

	requestDuration, err = meter.Float64Histogram(
		"claran_post_request_duration_seconds",
		metric.WithDescription("请求耗时"),
		metric.WithUnit("s"),
	)
	if err != nil {
		zap.L().Fatal("× 创建requestDuration失败",
			zap.Error(err),
		)
	}

	errorCounter, err = meter.Int64Counter(
		"claran_post_errors_total",
		metric.WithDescription("总错误数"),
	)
	if err != nil {
		zap.L().Fatal("× 创建errorCounter失败",
			zap.Error(err),
		)
	}
}

// PostServiceImpl implements the last service interface defined in the IDL.
type PostServiceImpl struct {
	PostService service.PostService
}

func NewPostServiceImpl(postService service.PostService) *PostServiceImpl {
	initMetrics()
	zap.L().Info("√ 初始化PostService处理器成功")
	return &PostServiceImpl{PostService: postService}
}

// CreatePost implements the PostServiceImpl interface.
func (s *PostServiceImpl) CreatePost(ctx context.Context, req *post.CreatePostReq) (resp *post.CreatePostResp, err error) {
	startTime := time.Now()
	zap.L().Info("开始处理CreatePost请求",
		zap.String("post_name", req.PostName),
		zap.Int64("user_id", req.UserID),
		zap.Int("content_length", len(req.Content)),
	)

	// 记录指标
	defer func() {
		duration := time.Since(startTime).Seconds()
		status := getstatus(err)

		requestDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("method", "CreatePost"),
				attribute.String("status", status),
			))

		requestCounter.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("method", "CreatePost"),
				attribute.String("status", status),
			))

		if err != nil {
			errorCounter.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("method", "CreatePost"),
					attribute.String("status", status),
				))
		}

		zap.L().Info("CreatePost请求处理完成",
			zap.Duration("duration", time.Since(startTime)),
			zap.String("status", status),
			zap.String("post_name", req.PostName),
		)
	}()

	// 创建span - 从全局获取 tracer
	tracer := otel.Tracer("post-service")
	spanCtx, span := tracer.Start(ctx, "handler.CreatePost",
		trace.WithAttributes(
			attribute.String("post_name", req.PostName),
		))
	defer span.End()

	posts, err := s.PostService.CreatePost(spanCtx, req.PostName, req.UserID, req.Content)
	if err != nil {
		zap.L().Error("× 执行创建文章服务失败",
			zap.Error(err),
			zap.String("handler", "CreatePost"),
			zap.String("post_name", req.PostName),
			zap.Int64("user_id", req.UserID),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "执行创建文章服务失败："+err.Error())
		span.SetAttributes(attribute.Bool("service.success", false))
		return nil, err
	}
	zap.L().Info("√ 创建文章服务执行成功",
		zap.Int64("post_id", posts.PostID),
		zap.String("post_title", posts.Title),
		zap.Int64("owner_id", posts.Owner),
	)

	span.SetAttributes(
		attribute.String("post_title", posts.Title),
		attribute.String("post_id", strconv.Itoa(int(posts.PostID))),
		attribute.String("content", posts.Content),
		attribute.String("owner_id", strconv.Itoa(int(posts.Owner))),
		attribute.Bool("service.success", true),
	)

	resp = &post.CreatePostResp{
		Success: true,
		PostID:  posts.PostID, // 乱写的，前期没构建好导致的
		Msg:     "success",
	}

	return resp, nil
}

// DeletePost implements the PostServiceImpl interface.
func (s *PostServiceImpl) DeletePost(ctx context.Context, req *post.DeletePostReq) (resp *post.DeletePostResp, err error) {
	startTime := time.Now()
	zap.L().Info("开始处理DeletePost请求",
		zap.Int64("post_id", req.PostID),
		zap.Int64("user_id", req.UserID),
	)

	// 记录指标
	defer func() {
		duration := time.Since(startTime).Seconds()
		status := getstatus(err)

		requestDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("method", "DeletePost"),
				attribute.String("status", status),
			))

		requestCounter.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("method", "DeletePost"),
				attribute.String("status", status),
			))

		if err != nil {
			errorCounter.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("method", "DeletePost"),
					attribute.String("status", status),
				))
		}

		zap.L().Info("DeletePost请求处理完成",
			zap.Duration("duration", time.Since(startTime)),
			zap.String("status", status),
			zap.Int64("post_id", req.PostID),
		)
	}()

	// 创建span - 从全局获取 tracer
	tracer := otel.Tracer("post-service")
	spanCtx, span := tracer.Start(ctx, "handler.DeletePost",
		trace.WithAttributes(
			attribute.String("post_id", strconv.Itoa(int(req.PostID))),
		))
	defer span.End()

	err = s.PostService.DeletePost(spanCtx, req.PostID)
	if err != nil {
		zap.L().Error("× 执行删除文章服务失败",
			zap.Error(err),
			zap.String("handler", "DeletePost"),
			zap.Int64("post_id", req.PostID),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "执行删除文章服务失败："+err.Error())
		span.SetAttributes(attribute.Bool("service.success", false))
		return nil, err
	}
	zap.L().Info("√ 删除文章服务执行成功",
		zap.Int64("post_id", req.PostID),
	)

	resp = &post.DeletePostResp{
		Success: true,
		Msg:     "success",
	}

	span.SetAttributes(
		attribute.Bool("service.success", true),
	)

	return resp, nil
}

func getstatus(err error) string {
	if err == nil {
		return "success"
	}
	return "error"
}
