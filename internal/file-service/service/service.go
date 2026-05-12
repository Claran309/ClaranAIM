package service

import (
	"ClaranAIM/internal/file-service/dao"
	"ClaranAIM/internal/file-service/model"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type FileService interface {
	UploadFile(ctx context.Context, fileName, fileType string, fileSize int64, contentType string, uploaderID int64, fileData io.Reader) (*model.FileRecord, error)
	GetFile(ctx context.Context, fileID string) (*model.FileRecord, error)
	DeleteFile(ctx context.Context, fileID string, operatorID int64) error
	ListFiles(ctx context.Context, uploaderID int64, fileType string, limit, offset int64) ([]model.FileRecord, int64, error)
	GetFileReader(ctx context.Context, fileID string) (io.ReadCloser, *model.FileRecord, error)
}

type fileServiceImpl struct {
	repo          dao.FileRepository
	storageDir    string
	minioClient   *minio.Client
	minioBucket   string
	useMinio      bool
	minioEndpoint string
}

func NewFileService(repo dao.FileRepository, storageDir, minioEndpoint, minioAccessKey, minioSecretKey, minioBucket string, useMinio bool) FileService {
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		log.Printf("创建存储目录失败: %v", err)
	}

	var minioClient *minio.Client
	if useMinio && minioEndpoint != "" {
		var err error
		minioClient, err = minio.New(minioEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
			Secure: false,
		})
		if err != nil {
			log.Printf("MinIO客户端初始化失败，将使用本地存储: %v", err)
			useMinio = false
		} else {
			ctx := context.Background()
			exists, err := minioClient.BucketExists(ctx, minioBucket)
			if err != nil {
				log.Printf("检查MinIO Bucket失败: %v", err)
			} else if !exists {
				if err := minioClient.MakeBucket(ctx, minioBucket, minio.MakeBucketOptions{}); err != nil {
					log.Printf("创建MinIO Bucket失败: %v", err)
				} else {
					log.Printf("MinIO Bucket '%s' 创建成功", minioBucket)
				}
			}
			log.Printf("MinIO连接成功，Bucket: %s", minioBucket)
		}
	}

	return &fileServiceImpl{
		repo:          repo,
		storageDir:    storageDir,
		minioClient:   minioClient,
		minioBucket:   minioBucket,
		useMinio:      useMinio,
		minioEndpoint: minioEndpoint,
	}
}

func (s *fileServiceImpl) UploadFile(ctx context.Context, fileName, fileType string, fileSize int64, contentType string, uploaderID int64, fileData io.Reader) (*model.FileRecord, error) {
	if fileName == "" {
		return nil, errors.New("文件名不能为空")
	}
	if fileType == "" {
		fileType = "file"
	}

	fileID := uuid.New().String()
	ext := filepath.Ext(fileName)
	objectName := filepath.Join(fileType, fileID+ext)
	objectName = filepath.ToSlash(objectName)

	var fileURL string
	var actualSize int64

	if s.useMinio && s.minioClient != nil {
		uploadInfo, err := s.minioClient.PutObject(ctx, s.minioBucket, objectName, fileData, -1, minio.PutObjectOptions{
			ContentType: contentType,
		})
		if err != nil {
			return nil, fmt.Errorf("上传到MinIO失败: %w", err)
		}
		actualSize = uploadInfo.Size
		fileURL = fmt.Sprintf("http://%s/%s/%s", s.minioEndpoint, s.minioBucket, objectName)
		log.Printf("文件上传到MinIO: %s (%d bytes)", objectName, actualSize)
	} else {
		fullPath := filepath.Join(s.storageDir, objectName)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return nil, fmt.Errorf("创建目录失败: %w", err)
		}

		dst, err := os.Create(fullPath)
		if err != nil {
			return nil, fmt.Errorf("创建文件失败: %w", err)
		}
		defer dst.Close()

		written, err := io.Copy(dst, fileData)
		if err != nil {
			os.Remove(fullPath)
			return nil, fmt.Errorf("写入文件失败: %w", err)
		}
		actualSize = written
		fileURL = fmt.Sprintf("/files/%s", objectName)
	}

	record := &model.FileRecord{
		FileID:      fileID,
		FileName:    fileName,
		FileType:    fileType,
		FileSize:    actualSize,
		ContentType: contentType,
		FileURL:     fileURL,
		UploaderID:  uploaderID,
	}

	if err := s.repo.CreateFile(ctx, record); err != nil {
		if !s.useMinio {
			fullPath := filepath.Join(s.storageDir, objectName)
			os.Remove(fullPath)
		}
		return nil, err
	}

	log.Printf("文件上传成功: %s (%d bytes), file_id=%s", fileName, actualSize, fileID)
	return record, nil
}

func (s *fileServiceImpl) GetFile(ctx context.Context, fileID string) (*model.FileRecord, error) {
	record, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errors.New("文件不存在")
	}
	return record, nil
}

func (s *fileServiceImpl) DeleteFile(ctx context.Context, fileID string, operatorID int64) error {
	record, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if record == nil {
		return errors.New("文件不存在")
	}
	if record.UploaderID != operatorID {
		return errors.New("只能删除自己上传的文件")
	}

	objectName := filepath.Join(record.FileType, record.FileID+filepath.Ext(record.FileName))
	objectName = filepath.ToSlash(objectName)

	if s.useMinio && s.minioClient != nil {
		if err := s.minioClient.RemoveObject(ctx, s.minioBucket, objectName, minio.RemoveObjectOptions{}); err != nil {
			log.Printf("从MinIO删除文件失败: %v", err)
		}
	} else {
		fullPath := filepath.Join(s.storageDir, objectName)
		os.Remove(fullPath)
	}

	return s.repo.DeleteFile(ctx, fileID)
}

func (s *fileServiceImpl) ListFiles(ctx context.Context, uploaderID int64, fileType string, limit, offset int64) ([]model.FileRecord, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.ListFiles(ctx, uploaderID, fileType, limit, offset)
}

func (s *fileServiceImpl) GetFileReader(ctx context.Context, fileID string) (io.ReadCloser, *model.FileRecord, error) {
	record, err := s.GetFile(ctx, fileID)
	if err != nil {
		return nil, nil, err
	}

	objectName := filepath.Join(record.FileType, record.FileID+filepath.Ext(record.FileName))
	objectName = filepath.ToSlash(objectName)

	if s.useMinio && s.minioClient != nil {
		obj, err := s.minioClient.GetObject(ctx, s.minioBucket, objectName, minio.GetObjectOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("从MinIO获取文件失败: %w", err)
		}
		return obj, record, nil
	}

	fullPath := filepath.Join(s.storageDir, objectName)
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, nil, fmt.Errorf("文件不存在或已被删除: %w", err)
	}
	return file, record, nil
}

func GeneratePresignedURL(endpoint, bucket, objectName string, expiry time.Duration) string {
	return fmt.Sprintf("http://%s/%s/%s", endpoint, bucket, objectName)
}
