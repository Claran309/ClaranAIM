package dao

import (
	"context"
	"errors"
	"microservicesDemo/L7-ELK/internal/post-service/model"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB(dsn string) (*gorm.DB, error) {
	zap.L().Info("开始初始化数据库连接")

	db, err := gorm.Open(mysql.Open(dsn))
	if err != nil {
		zap.L().Error("× 数据库连接失败",
			zap.Error(err),
			zap.String("component", "dao"),
		)
		return nil, errors.New("数据库连接失败：" + err.Error())
	}

	zap.L().Info("√ 数据库连接成功",
		zap.String("component", "dao"),
	)
	return db, nil
}

type PostRepository interface {
	AddPost(ctx context.Context, post *model.Post) error
	DeletePost(ctx context.Context, postID int64) error
}

type postRepositoryImpl struct {
	db *gorm.DB
}

func NewPostRepositoryImpl(db *gorm.DB) PostRepository {
	zap.L().Info("开始初始化PostRepository")

	err := db.AutoMigrate(&model.Post{})
	if err != nil {
		zap.L().Fatal("× 自动迁移Post模型失败",
			zap.Error(err),
			zap.String("component", "dao"),
		)
		return nil
	}

	zap.L().Info("√ 自动迁移Post模型成功",
		zap.String("component", "dao"),
	)

	zap.L().Info("√ 初始化PostRepository成功",
		zap.String("component", "dao"),
	)
	return &postRepositoryImpl{db: db}
}

func (p *postRepositoryImpl) AddPost(ctx context.Context, post *model.Post) error {
	zap.L().Info("开始执行AddPost操作",
		zap.Int64("user_id", post.Owner),
		zap.String("title", post.Title),
		zap.String("component", "dao"),
	)

	tracer := otel.Tracer("post-service")
	ctx, span := tracer.Start(ctx, "dao.AddPost")
	defer span.End()

	span.SetAttributes(attribute.Bool("dao.success", true))

	err := p.db.Create(post).Error
	if err != nil {
		zap.L().Error("× 添加文章失败",
			zap.Error(err),
			zap.String("component", "dao"),
			zap.Int64("user_id", post.Owner),
			zap.String("title", post.Title),
		)
		return err
	}

	zap.L().Info("√ 添加文章成功",
		zap.Int64("post_id", post.PostID),
		zap.Int64("user_id", post.Owner),
		zap.String("title", post.Title),
		zap.String("component", "dao"),
	)

	return nil
}

func (p *postRepositoryImpl) DeletePost(ctx context.Context, postID int64) error {
	zap.L().Info("开始执行DeletePost操作",
		zap.Int64("post_id", postID),
		zap.String("component", "dao"),
	)

	tracer := otel.Tracer("post-service")
	ctx, span := tracer.Start(ctx, "dao.DeletePost")
	defer span.End()

	span.SetAttributes(attribute.Bool("dao.success", true))

	err := p.db.Delete(&model.Post{}, postID).Error
	if err != nil {
		zap.L().Error("× 删除文章失败",
			zap.Error(err),
			zap.String("component", "dao"),
			zap.Int64("post_id", postID),
		)
		return err
	}

	zap.L().Info("√ 删除文章成功",
		zap.Int64("post_id", postID),
		zap.String("component", "dao"),
	)

	return nil
}
