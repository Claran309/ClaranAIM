// Package dao contains file-service persistence adapters.
//
// File-service stores metadata only. Binary object reads/writes are handled by
// the service layer or api-gateway depending on the upload path.
package dao

import (
	"ClaranAIM/internal/file-service/model"
	"context"
	"errors"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB opens MySQL and performs non-destructive migration for file metadata.
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

// FileRepository defines all metadata persistence operations used by file-service.
type FileRepository interface {
	CreateFile(ctx context.Context, file *model.FileRecord) error
	GetFileByID(ctx context.Context, fileID string) (*model.FileRecord, error)
	DeleteFile(ctx context.Context, fileID string) error
	ListFiles(ctx context.Context, uploaderID int64, fileType string, limit, offset int64) ([]model.FileRecord, int64, error)
}

type fileRepositoryImpl struct {
	db *gorm.DB
}

// NewFileRepo creates a GORM-backed file repository.
func NewFileRepo(db *gorm.DB) FileRepository {
	return &fileRepositoryImpl{db: db}
}

// CreateFile inserts one file metadata record.
func (r *fileRepositoryImpl) CreateFile(ctx context.Context, file *model.FileRecord) error {
	return r.db.WithContext(ctx).Create(file).Error
}

// GetFileByID returns metadata by public file_id, or nil when it does not exist.
func (r *fileRepositoryImpl) GetFileByID(ctx context.Context, fileID string) (*model.FileRecord, error) {
	var file model.FileRecord
	err := r.db.WithContext(ctx).Where("file_id = ?", fileID).First(&file).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &file, err
}

// DeleteFile removes metadata for one file_id.
func (r *fileRepositoryImpl) DeleteFile(ctx context.Context, fileID string) error {
	return r.db.WithContext(ctx).Where("file_id = ?", fileID).Delete(&model.FileRecord{}).Error
}

// ListFiles returns a paginated metadata page and total count for one uploader.
func (r *fileRepositoryImpl) ListFiles(ctx context.Context, uploaderID int64, fileType string, limit, offset int64) ([]model.FileRecord, int64, error) {
	var files []model.FileRecord
	var total int64

	query := r.db.WithContext(ctx).Model(&model.FileRecord{}).Where("uploader_id = ?", uploaderID)
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
