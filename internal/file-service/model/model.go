// Package model 定义 file-service 的持久化模型。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// FileRecord 保存一个已上传对象的元数据。
// 文件实体可以位于本地磁盘或 MinIO；FileURL 指向实际对象，FileID 是聊天媒体消息使用的稳定引用 ID。
type FileRecord struct {
	ID          int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	FileID      string    `json:"file_id" gorm:"uniqueIndex;size:64;not null"`
	FileName    string    `json:"file_name" gorm:"size:255;not null"`
	FileType    string    `json:"file_type" gorm:"size:20;not null"`
	FileSize    int64     `json:"file_size" gorm:"not null"`
	ContentType string    `json:"content_type" gorm:"size:128"`
	FileURL     string    `json:"file_url" gorm:"size:512;not null"`
	UploaderID  int64     `json:"uploader_id" gorm:"index;not null"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// BeforeCreate 在插入文件元数据前补充分布式雪花主键。
func (f *FileRecord) BeforeCreate(tx *gorm.DB) error {
	if f.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	if err != nil {
		return err
	}
	f.ID = id
	return nil
}

// TableName 固定文件元数据表名。
func (FileRecord) TableName() string {
	return "file_records"
}
