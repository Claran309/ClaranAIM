// Package model defines file-service persistence models.
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// FileRecord stores metadata for one uploaded object.
//
// The object itself may live on local disk or MinIO; FileURL points to that
// object while FileID is the stable ID used by chat media payloads.
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

// BeforeCreate assigns a snowflake primary key before inserting metadata.
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

// TableName keeps the metadata table name explicit.
func (FileRecord) TableName() string {
	return "file_records"
}
