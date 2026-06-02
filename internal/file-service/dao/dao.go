// Package dao 包含 file-service 的持久化适配器。
// file-service 只保存文件元数据；二进制对象读写由 service 层或 api-gateway 根据上传路径处理。
package dao

import (
	"ClaranAIM/internal/file-service/model"
	"context"
	"errors"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 打开 MySQL，并对文件元数据表执行非破坏性迁移。
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	models := []interface{}{
		&model.FileRecord{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}

	return db, nil
}

// FileRepository 定义 file-service 使用的全部元数据持久化操作。
type FileRepository interface {
	CreateFile(ctx context.Context, file *model.FileRecord) error
	GetFileByID(ctx context.Context, fileID string) (*model.FileRecord, error)
	DeleteFile(ctx context.Context, fileID string) error
	ListFiles(ctx context.Context, uploaderID int64, fileType string, limit, offset int64) ([]model.FileRecord, int64, error)
}

// fileRepositoryImpl 是基于 GORM 的文件元数据仓储实现。
type fileRepositoryImpl struct {
	db *gorm.DB
}

// NewFileRepo 创建基于 GORM 的文件仓储。
func NewFileRepo(db *gorm.DB) FileRepository {
	return &fileRepositoryImpl{db: db}
}

// CreateFile 插入一条文件元数据记录。
func (r *fileRepositoryImpl) CreateFile(ctx context.Context, file *model.FileRecord) error {
	return r.db.WithContext(ctx).Create(file).Error
}

// GetFileByID 根据公开 file_id 查询元数据，不存在时返回 nil。
func (r *fileRepositoryImpl) GetFileByID(ctx context.Context, fileID string) (*model.FileRecord, error) {
	var file model.FileRecord
	err := r.db.WithContext(ctx).Where("file_id = ?", fileID).First(&file).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &file, err
}

// DeleteFile 删除某个 file_id 对应的元数据。
func (r *fileRepositoryImpl) DeleteFile(ctx context.Context, fileID string) error {
	return r.db.WithContext(ctx).Where("file_id = ?", fileID).Delete(&model.FileRecord{}).Error
}

// ListFiles 返回文件分页元数据和总数。
// uploaderID > 0 时按上传者裁剪，供普通用户文件列表使用；uploaderID == 0 时返回全局文件，
// 只允许 admin-service 这类受保护的内部管理入口调用。
func (r *fileRepositoryImpl) ListFiles(ctx context.Context, uploaderID int64, fileType string, limit, offset int64) ([]model.FileRecord, int64, error) {
	var files []model.FileRecord
	var total int64

	query := r.db.WithContext(ctx).Model(&model.FileRecord{})
	if uploaderID > 0 {
		query = query.Where("uploader_id = ?", uploaderID)
	}
	if fileType != "" {
		query = query.Where("file_type = ?", fileType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("id DESC").Limit(int(limit)).Offset(int(offset)).Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}
