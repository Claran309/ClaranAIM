package dao

import (
	"context"
	"errors"
	"microservicesDemo/L7-ELK/internal/user-service/model"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB(dsn string) (*gorm.DB, error) {
	zap.L().Info("开始初始化数据库连接",
		zap.String("component", "dao"),
	)

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

type UserRepository interface {
	AddUser(ctx context.Context, user *model.User) error
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
}

type userRepositoryImpl struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) UserRepository {
	zap.L().Info("开始初始化UserRepository",
		zap.String("component", "dao"),
	)

	err := db.AutoMigrate(&model.User{})
	if err != nil {
		zap.L().Fatal("× 自动迁移User模型失败",
			zap.Error(err),
			zap.String("component", "dao"),
		)
		return nil
	}

	zap.L().Info("√ 自动迁移User模型成功",
		zap.String("component", "dao"),
	)

	zap.L().Info("√ 初始化UserRepository成功",
		zap.String("component", "dao"),
	)
	return &userRepositoryImpl{db: db}
}

func (repo *userRepositoryImpl) AddUser(ctx context.Context, user *model.User) error {
	zap.L().Info("开始执行AddUser操作",
		zap.String("username", user.Username),
		zap.String("email", user.Email),
		zap.String("component", "dao"),
	)

	tracer := otel.Tracer("user-service")
	ctx, span := tracer.Start(ctx, "dao.AddUser")
	defer span.End()

	var exist bool
	repo.db.Where("username = ?", user.Username).First(&exist)
	if exist != false {
		zap.L().Error("× 用户名已存在",
			zap.String("component", "dao"),
			zap.String("username", user.Username),
		)
		span.RecordError(errors.New("用户名已存在"))
		span.SetStatus(codes.Error, "用户名已存在")
		span.SetAttributes(attribute.Bool("dao.success", false))
		return errors.New("用户名已存在")
	}

	exist = false
	repo.db.Where("email = ?", user.Email).First(&exist)
	if exist != false {
		zap.L().Error("× 邮箱已存在",
			zap.String("component", "dao"),
			zap.String("email", user.Email),
		)
		span.RecordError(errors.New("邮箱已存在"))
		span.SetStatus(codes.Error, "邮箱已存在")
		span.SetAttributes(attribute.Bool("dao.success", false))
		return errors.New("邮箱已存在")
	}

	err := repo.db.Create(user).Error
	if err != nil {
		zap.L().Error("× 创建用户失败",
			zap.Error(err),
			zap.String("component", "dao"),
			zap.String("username", user.Username),
			zap.String("email", user.Email),
		)
		return err
	}

	zap.L().Info("√ 创建用户成功",
		zap.Int64("user_id", user.UserID),
		zap.String("username", user.Username),
		zap.String("email", user.Email),
		zap.String("component", "dao"),
	)

	span.SetAttributes(attribute.Bool("dao.success", true))
	return nil
}

func (repo *userRepositoryImpl) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	zap.L().Info("开始执行GetUserByUsername操作",
		zap.String("username", username),
		zap.String("component", "dao"),
	)

	tracer := otel.Tracer("user-service")
	ctx, span := tracer.Start(ctx, "dao.GetUserByUsername")
	defer span.End()

	var user model.User
	result := repo.db.Where("username = ?", username).First(&user)

	if result.Error != nil {
		zap.L().Error("× 查找用户失败",
			zap.Error(result.Error),
			zap.String("component", "dao"),
			zap.String("username", username),
		)
		return nil, result.Error
	}

	zap.L().Info("√ 查找用户成功",
		zap.Int64("user_id", user.UserID),
		zap.String("username", user.Username),
		zap.String("email", user.Email),
		zap.String("component", "dao"),
	)

	span.SetAttributes(attribute.Bool("dao.success", true))
	return &user, nil
}
