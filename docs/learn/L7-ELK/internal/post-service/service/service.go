package service

import (
	"context"
	"errors"
	"fmt"
	"microservicesDemo/L7-ELK/internal/post-service/dao"
	"microservicesDemo/L7-ELK/internal/post-service/model"
	mq "microservicesDemo/L7-ELK/pkg/mq/kafka"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/common/json"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

type PostService interface {
	//业务方法
	CreatePost(ctx context.Context, title string, userID int64, content string) (*model.Post, error)
	DeletePost(ctx context.Context, postID int64) error
	// 消费者方法
	StartConsumer(topic, groupID string) error
	StopConsumer()
}

type postServiceImpl struct {
	PostRepo      dao.PostRepository
	KafkaProducer *mq.Producer
	KafkaConsumer *mq.Consumer
	StopChan      chan struct{}
}

func NewPostServiceImpl(PostRepo dao.PostRepository, kafkaProducer *mq.Producer, kafkaConsumer *mq.Consumer) PostService {
	return &postServiceImpl{PostRepo: PostRepo, KafkaProducer: kafkaProducer, KafkaConsumer: kafkaConsumer, StopChan: make(chan struct{})}
}

// 启动消费者
func (s *postServiceImpl) StartConsumer(topic, groupID string) error {
	// 创建消费者
	consumer, err := mq.NewConsumer(topic, groupID)
	if err != nil {
		return fmt.Errorf("创建消费者失败: %v", err)
	}
	s.KafkaConsumer = consumer

	// 启动goroutine消费
	go s.consumeLoop()
	zap.L().Info("√ 消费者启动成功",
		zap.String("topic", topic),
		zap.String("group", groupID),
	)
	return nil
}

// 消费者主循环
func (s *postServiceImpl) consumeLoop() {
	for {
		select {
		case <-s.StopChan:
			zap.L().Info("√ 消费者停止")
			return
		default:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			// 接收消息
			msg, err := s.KafkaConsumer.Receive(ctx)
			cancel()
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					// 读取超时，继续循环
					continue
				}
				zap.L().Error("× 接收消息失败",
					zap.Error(err),
					zap.String("service", "post-service"),
				)
				time.Sleep(1 * time.Second)
				continue
			}
			if msg == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			// 处理消息
			go s.processMessage(msg)
		}
	}
}

// 处理消息
func (s *postServiceImpl) processMessage(msg *mq.Message) {
	// 解析消息
	var data map[string]interface{}
	if err := json.Unmarshal(msg.Value, &data); err != nil {
		zap.L().Error("× 解析消息失败",
			zap.Error(err),
			zap.String("service", "post-service"),
		)
		return
	}
	// 获取事件类型
	eventType, ok := data["event_type"].(string)
	if !ok {
		zap.L().Warn("× 消息格式错误: 缺少event_type",
			zap.String("service", "post-service"),
		)
		return
	}
	// 根据事件类型调用已有的Service方法
	ctx := context.Background()
	switch eventType {
	case "USER_REGISTER":
		s.CreatePost(ctx, data["Welcome!"].(string), data["user_id"].(int64), data["content"].(string))
		zap.L().Info("√ 处理USER_REGISTER事件",
			zap.String("service", "post-service"),
			zap.String("event_type", eventType),
		)
	case "USER_LOGIN":
		s.CreatePost(ctx, data["Login!"].(string), data["user_id"].(int64), data["content"].(string))
		zap.L().Info("√ 处理USER_LOGIN事件",
			zap.String("service", "post-service"),
			zap.String("event_type", eventType),
		)
	default:
		zap.L().Warn("× 未知事件类型",
			zap.String("service", "post-service"),
			zap.String("event_type", eventType),
		)
	}
}

// 停止消费者
func (s *postServiceImpl) StopConsumer() {
	if s.KafkaConsumer != nil {
		close(s.StopChan)
		s.KafkaConsumer.Close()
		zap.L().Info("√ 消费者已停止",
			zap.String("service", "post-service"),
		)
	}
}

func (s *postServiceImpl) CreatePost(ctx context.Context, title string, userID int64, content string) (*model.Post, error) {
	zap.L().Info("开始执行CreatePost服务",
		zap.String("title", title),
		zap.Int64("user_id", userID),
		zap.Int("content_length", len(content)),
	)

	tracer := otel.Tracer("post-service")
	spanCtx, span := tracer.Start(ctx, "service.CreatePost")
	defer span.End()

	var post = model.Post{
		Title:   title,
		Content: content,
		Owner:   userID,
	}

	err := s.PostRepo.AddPost(spanCtx, &post)
	if err != nil {
		zap.L().Error("× 添加文章失败",
			zap.Error(err),
			zap.String("service", "post-service"),
			zap.String("title", title),
			zap.Int64("user_id", userID),
		)
		span.RecordError(errors.New("添加文章失败: " + err.Error()))
		span.SetStatus(codes.Error, "添加文章失败: "+err.Error())
		span.SetAttributes(attribute.Bool("dao.success", false))
		return nil, err
	}
	zap.L().Info("√ 添加文章成功",
		zap.String("service", "post-service"),
		zap.Int64("post_id", post.PostID),
		zap.Int64("user_id", userID),
	)

	// 生产者发送消息
	if s.KafkaProducer != nil {
		eventData, _ := json.Marshal(map[string]interface{}{
			"event_type":    "POST_CREATED", // 明确的事件类型
			"post_id":       post.PostID,
			"user_id":       userID,
			"title":         post.Title,
			"content":       post.Content,
			"registered_at": time.Now(),
		})

		err := s.KafkaProducer.SendPostEvent(ctx, fmt.Sprintf("%d", post.PostID), eventData)
		if err != nil {
			zap.L().Error("× 发送添加文章事件失败",
				zap.Error(err),
				zap.String("service", "post-service"),
				zap.Int64("post_id", post.PostID),
			)
			span.RecordError(errors.New("发送添加文章事件失败:user_id:%d" + strconv.Itoa(int(post.PostID))))
			span.SetStatus(codes.Error, "发送添加文章事件失败:user_id:%d"+strconv.Itoa(int(post.PostID))+err.Error())
			span.SetAttributes(attribute.Bool("kafka.success", false))
		} else {
			zap.L().Info("√ 发送帖子创建事件成功",
				zap.String("service", "post-service"),
				zap.Int64("post_id", post.PostID),
			)
		}
	}

	span.SetAttributes(attribute.Bool("kafka.success", true))
	return &post, nil
}

func (s *postServiceImpl) DeletePost(ctx context.Context, postID int64) error {
	zap.L().Info("开始执行DeletePost服务",
		zap.Int64("post_id", postID),
	)

	tracer := otel.Tracer("post-service")
	ctx, span := tracer.Start(ctx, "service.DeletePost")
	defer span.End()

	span.SetAttributes(attribute.Bool("kafka.success", true))

	err := s.PostRepo.DeletePost(ctx, postID)
	if err != nil {
		zap.L().Error("× 删除文章失败",
			zap.Error(err),
			zap.String("service", "post-service"),
			zap.Int64("post_id", postID),
		)
		return err
	}

	zap.L().Info("√ 删除文章成功",
		zap.String("service", "post-service"),
		zap.Int64("post_id", postID),
	)

	return nil
}
