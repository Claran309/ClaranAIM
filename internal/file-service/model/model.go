package model

import "time"

type FileRecord struct {
	ID          int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	FileID      string    `json:"file_id" gorm:"uniqueIndex;size:64;not null"`
	FileName    string    `json:"file_name" gorm:"size:255;not null"`
	FileType    string    `json:"file_type" gorm:"size:20;not null"`
	FileSize    int64     `json:"file_size" gorm:"not null"`
	ContentType string    `json:"content_type" gorm:"size:128"`
	FileURL     string    `json:"file_url" gorm:"size:512;not null"`
	UploaderID  int64     `json:"uploader_id" gorm:"index;not null"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (FileRecord) TableName() string {
	return "file_records"
}
