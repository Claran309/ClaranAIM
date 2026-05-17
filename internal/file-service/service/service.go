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
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// FileService defines the file-domain operations.
//
// The current deployment may store bytes either on local disk or MinIO, while
// metadata is always persisted through file-service. Messages should store only
// file references, never raw binary payloads.
type FileService interface {
	UploadFile(ctx context.Context, fileName, fileType string, fileSize int64, contentType string, uploaderID int64, fileData io.Reader) (*model.FileRecord, error)
	SaveFileRecord(ctx context.Context, fileName, fileType string, fileSize int64, contentType, fileURL string, uploaderID int64) (*model.FileRecord, error)
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

// NewFileService initializes the file service and prepares the configured object
// store. If MinIO initialization fails, the service falls back to local storage
// so development uploads still work.
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

// UploadFile stores a binary stream and creates its metadata record.
//
// This method is primarily useful for RPC callers that can stream the file body
// directly to file-service. The HTTP gateway usually stores bytes first and then
// calls SaveFileRecord to avoid forwarding multipart streams over RPC.
func (s *fileServiceImpl) UploadFile(ctx context.Context, fileName, fileType string, fileSize int64, contentType string, uploaderID int64, fileData io.Reader) (*model.FileRecord, error) {
	if fileName == "" {
		return nil, errors.New("文件名不能为空")
	}
	fileID := uuid.New().String()
	ext := filepath.Ext(fileName)
	objectName, cleanFileType, err := buildUploadObjectName(fileType, fileID, ext)
	if err != nil {
		return nil, err
	}

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
		FileType:    cleanFileType,
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

// GetFile returns metadata for one file ID.
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

// DeleteFile removes metadata and attempts to remove the backing object.
// Only the uploader may delete the file in the current permission model.
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

	objectName, err := objectNameFromFileURL(record.FileURL)
	if err != nil {
		return err
	}

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

// ListFiles returns paginated file metadata for one uploader.
func (s *fileServiceImpl) ListFiles(ctx context.Context, uploaderID int64, fileType string, limit, offset int64) ([]model.FileRecord, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.ListFiles(ctx, uploaderID, fileType, limit, offset)
}

// GetFileReader opens the backing object for download/preview and returns its
// metadata alongside the stream.
func (s *fileServiceImpl) GetFileReader(ctx context.Context, fileID string) (io.ReadCloser, *model.FileRecord, error) {
	record, err := s.GetFile(ctx, fileID)
	if err != nil {
		return nil, nil, err
	}

	objectName, err := objectNameFromFileURL(record.FileURL)
	if err != nil {
		return nil, nil, err
	}

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

// GeneratePresignedURL returns a public-style object URL placeholder.
// The expiry argument is kept for future MinIO presigned URL support.
func GeneratePresignedURL(endpoint, bucket, objectName string, expiry time.Duration) string {
	return fmt.Sprintf("http://%s/%s/%s", endpoint, bucket, objectName)
}

func buildUploadObjectName(fileType, fileID, ext string) (string, string, error) {
	if fileType == "" {
		fileType = "file"
	}
	normalizedType := filepath.ToSlash(fileType)
	if strings.HasPrefix(normalizedType, "/") {
		return "", "", errors.New("无效的文件类型")
	}
	segments := strings.Split(normalizedType, "/")
	cleanSegments := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		if segment == "." || segment == ".." {
			return "", "", errors.New("无效的文件类型")
		}
		cleanSegments = append(cleanSegments, segment)
	}
	if len(cleanSegments) == 0 {
		return "", "", errors.New("无效的文件类型")
	}
	cleanType := path.Join(cleanSegments...)
	return path.Join(cleanType, fileID+ext), cleanType, nil
}

func objectNameFromFileURL(fileURL string) (string, error) {
	if fileURL == "" {
		return "", errors.New("文件地址为空")
	}

	var objectName string
	if strings.HasPrefix(fileURL, "/files/") {
		objectName = strings.TrimPrefix(fileURL, "/files/")
	} else if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") {
		parts := strings.SplitN(fileURL, "://", 2)
		if len(parts) != 2 {
			return "", errors.New("无效的文件地址")
		}
		segments := strings.SplitN(parts[1], "/", 3)
		if len(segments) < 3 {
			return "", errors.New("无效的文件地址")
		}
		objectName = segments[2]
	} else {
		return "", errors.New("不支持的文件地址")
	}

	objectName = path.Clean("/" + objectName)
	objectName = strings.TrimPrefix(objectName, "/")
	if objectName == "" || objectName == "." || strings.HasPrefix(objectName, "../") {
		return "", errors.New("无效的文件路径")
	}
	return objectName, nil
}

// SaveFileRecord persists metadata for an object already stored by the gateway.
func (s *fileServiceImpl) SaveFileRecord(ctx context.Context, fileName, fileType string, fileSize int64, contentType, fileURL string, uploaderID int64) (*model.FileRecord, error) {
	if fileName == "" {
		return nil, errors.New("文件名不能为空")
	}
	if fileType == "" {
		fileType = "file"
	}

	fileID := uuid.New().String()

	record := &model.FileRecord{
		FileID:      fileID,
		FileName:    fileName,
		FileType:    fileType,
		FileSize:    fileSize,
		ContentType: contentType,
		FileURL:     fileURL,
		UploaderID:  uploaderID,
	}

	if err := s.repo.CreateFile(ctx, record); err != nil {
		return nil, err
	}

	log.Printf("文件记录保存成功: %s (%d bytes), file_id=%s", fileName, fileSize, fileID)
	return record, nil
}
