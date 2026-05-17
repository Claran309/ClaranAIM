package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/logger"
	"ClaranAIM/pkg/response"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var minioClient *minio.Client
var minioBucket string
var useMinio bool
var minioEndpoint string
var storageDir string

// InitFileStorage prepares the binary object store used by the HTTP gateway.
// Metadata is persisted through file-service; the gateway owns the streaming
// boundary because browser uploads arrive here as multipart data.
func InitFileStorage(cfg *config.Config) {
	storageDir = cfg.Storage.Dir
	if storageDir == "" {
		storageDir = "./storage/source"
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		logger.Error("创建本地存储目录失败", "error", err)
	}

	if cfg.Minio.UseMinio && cfg.Minio.Endpoint != "" {
		mc, err := minio.New(cfg.Minio.Endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.Minio.AccessKey, cfg.Minio.SecretKey, ""),
			Secure: false,
		})
		if err != nil {
			logger.Error("MinIO客户端初始化失败，将使用本地存储", "error", err)
			useMinio = false
			return
		}

		ctx := context.Background()
		exists, err := mc.BucketExists(ctx, cfg.Minio.Bucket)
		if err != nil {
			logger.Warn("检查MinIO Bucket失败", "error", err)
		} else if !exists {
			if err := mc.MakeBucket(ctx, cfg.Minio.Bucket, minio.MakeBucketOptions{}); err != nil {
				logger.Error("创建MinIO Bucket失败", "error", err)
			} else {
				logger.Info("MinIO Bucket创建成功", "bucket", cfg.Minio.Bucket)
			}
		}

		minioClient = mc
		minioBucket = cfg.Minio.Bucket
		minioEndpoint = cfg.Minio.Endpoint
		useMinio = true
		logger.Info("文件上传使用MinIO", "bucket", minioBucket)
	} else {
		useMinio = false
		logger.Info("文件上传使用本地存储", "dir", storageDir)
	}
}

// FileHandler handles browser file upload, preview, download and metadata list
// endpoints. The gateway streams bytes because it owns HTTP multipart parsing;
// file-service stores metadata and authorization-relevant ownership fields.
type FileHandler struct{}

// NewFileHandler constructs the stateless file HTTP handler used by the router.
func NewFileHandler() *FileHandler {
	return &FileHandler{}
}

// UploadFile stores the binary payload first, then asks file-service to persist
// the metadata record. The returned file_id must come from file-service, because
// that is the ID later used by /file/download/:id and message media payloads.
func (h *FileHandler) UploadFile(ctx context.Context, c *app.RequestContext) {
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择文件")
		return
	}

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	fileType := c.DefaultPostForm("file_type", "file")

	src, err := file.Open()
	if err != nil {
		response.Error(c, "读取文件失败")
		return
	}
	defer src.Close()

	fileID := uuid.New().String()
	ext := filepath.Ext(file.Filename)
	objectName, err := buildUploadObjectName(fileType, fileID, ext)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var fileURL string
	var actualSize int64

	if useMinio && minioClient != nil {
		uploadInfo, err := minioClient.PutObject(ctx, minioBucket, objectName, src, -1, minio.PutObjectOptions{
			ContentType: file.Header.Get("Content-Type"),
		})
		if err != nil {
			logger.Error("上传到MinIO失败", "error", err, "object", objectName)
			response.Error(c, fmt.Sprintf("上传到MinIO失败: %v", err))
			return
		}
		actualSize = uploadInfo.Size
		fileURL = fmt.Sprintf("http://%s/%s/%s", minioEndpoint, minioBucket, objectName)
		logger.Info("文件上传到MinIO成功", "object", objectName, "size", actualSize)
	} else {
		fullPath := filepath.Join(storageDir, objectName)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			logger.Error("创建目录失败", "error", err)
			response.Error(c, fmt.Sprintf("创建目录失败: %v", err))
			return
		}

		dst, err := os.Create(fullPath)
		if err != nil {
			logger.Error("创建文件失败", "error", err)
			response.Error(c, fmt.Sprintf("创建文件失败: %v", err))
			return
		}
		defer dst.Close()

		written, err := io.Copy(dst, src)
		if err != nil {
			os.Remove(fullPath)
			logger.Error("写入文件失败", "error", err)
			response.Error(c, fmt.Sprintf("写入文件失败: %v", err))
			return
		}
		actualSize = written
		fileURL = fmt.Sprintf("/files/%s", objectName)
		logger.Info("文件上传到本地成功", "path", fullPath, "size", actualSize)
	}

	resp, err := client.FileClient.UploadFile(ctx, client.NewUploadFileReq(file.Filename, fileType, actualSize, file.Header.Get("Content-Type"), fileURL, id))
	if err != nil {
		logger.Error("文件元数据RPC调用失败", "error", err, "filename", file.Filename)
		cleanupUploadedObject(ctx, objectName)
		response.Error(c, fmt.Sprintf("文件已上传但元数据保存失败: %v", err))
		return
	}

	if resp == nil || !resp.Success {
		cleanupUploadedObject(ctx, objectName)
		msg := "未知错误"
		if resp != nil {
			msg = resp.Msg
		}
		logger.Error("文件元数据保存失败", "msg", msg, "filename", file.Filename)
		response.Error(c, fmt.Sprintf("文件已上传但元数据保存失败: %s", msg))
		return
	}

	logger.Info("文件上传完成", "file_id", fileID, "filename", file.Filename, "size", actualSize)
	response.Success(c, map[string]interface{}{
		"success":  true,
		"file_id":  resp.FileId,
		"file_url": fileURL,
		"filename": file.Filename,
		"size":     actualSize,
		"msg":      "上传成功",
	})
}

// GetFile returns metadata for one uploaded file without streaming the content.
func (h *FileHandler) GetFile(ctx context.Context, c *app.RequestContext) {
	fileID := c.Param("id")
	resp, err := client.FileClient.GetFile(ctx, client.NewGetFileReq(fileID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if !resp.Success {
		response.Error(c, resp.Msg)
		return
	}
	response.Success(c, resp)
}

// DownloadFile streams a file as an attachment.
func (h *FileHandler) DownloadFile(ctx context.Context, c *app.RequestContext) {
	h.serveFileByID(ctx, c, true)
}

// PreviewFile streams a file inline for chat image/audio preview.
func (h *FileHandler) PreviewFile(ctx context.Context, c *app.RequestContext) {
	h.serveFileByID(ctx, c, false)
}

func (h *FileHandler) serveFileByID(ctx context.Context, c *app.RequestContext, attachment bool) {
	fileID := c.Param("id")
	resp, err := client.FileClient.GetFile(ctx, client.NewGetFileReq(fileID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if !resp.Success {
		response.Error(c, resp.Msg)
		return
	}
	if strings.HasPrefix(resp.FileUrl, "http://") || strings.HasPrefix(resp.FileUrl, "https://") {
		if !useMinio || minioClient == nil {
			c.Redirect(http.StatusFound, []byte(resp.FileUrl))
			return
		}
		objectName, err := minioObjectNameFromURL(resp.FileUrl)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		obj, err := minioClient.GetObject(ctx, minioBucket, objectName, minio.GetObjectOptions{})
		if err != nil {
			logger.Error("读取MinIO对象失败", "error", err, "object", objectName)
			response.Error(c, "文件读取失败")
			return
		}
		defer obj.Close()
		if resp.ContentType != "" {
			c.SetContentType(resp.ContentType)
		}
		disposition := "inline"
		if attachment {
			disposition = "attachment"
		}
		c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, resp.FileName))
		if _, err := io.Copy(c, obj); err != nil {
			logger.Error("写出MinIO对象失败", "error", err, "object", objectName)
		}
		return
	}
	if !strings.HasPrefix(resp.FileUrl, "/files/") {
		response.BadRequest(c, "不支持的文件地址")
		return
	}

	// Local file URLs are stored as /files/<type>/<object>. Clean the path before
	// joining it with storageDir so a crafted URL cannot escape the storage root.
	relativePath := strings.TrimPrefix(resp.FileUrl, "/files/")
	cleanPath := filepath.Clean(filepath.FromSlash(relativePath))
	if cleanPath == "." || strings.HasPrefix(cleanPath, ".."+string(os.PathSeparator)) || filepath.IsAbs(cleanPath) {
		response.BadRequest(c, "无效的文件路径")
		return
	}

	fullPath := filepath.Join(storageDir, cleanPath)
	if _, err := os.Stat(fullPath); err != nil {
		response.Error(c, "文件不存在或已被删除")
		return
	}
	if resp.ContentType != "" {
		c.SetContentType(resp.ContentType)
	}
	disposition := "inline"
	if attachment {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, resp.FileName))
	c.File(fullPath)
}

func minioObjectNameFromURL(fileURL string) (string, error) {
	prefixes := []string{
		fmt.Sprintf("http://%s/%s/", minioEndpoint, minioBucket),
		fmt.Sprintf("https://%s/%s/", minioEndpoint, minioBucket),
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(fileURL, prefix) {
			objectName := strings.TrimPrefix(fileURL, prefix)
			objectName = path.Clean("/" + objectName)
			objectName = strings.TrimPrefix(objectName, "/")
			if objectName == "." || objectName == "" || strings.HasPrefix(objectName, "../") {
				return "", errors.New("无效的MinIO对象路径")
			}
			return objectName, nil
		}
	}
	return "", errors.New("不支持的MinIO文件地址")
}

func buildUploadObjectName(fileType, fileID, ext string) (string, error) {
	if fileType == "" {
		fileType = "file"
	}
	normalizedType := filepath.ToSlash(fileType)
	if strings.HasPrefix(normalizedType, "/") {
		return "", errors.New("无效的文件类型")
	}
	segments := strings.Split(normalizedType, "/")
	cleanSegments := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		if segment == "." || segment == ".." {
			return "", errors.New("无效的文件类型")
		}
		cleanSegments = append(cleanSegments, segment)
	}
	if len(cleanSegments) == 0 {
		return "", errors.New("无效的文件类型")
	}
	cleanType := path.Join(cleanSegments...)
	return path.Join(cleanType, fileID+ext), nil
}

func cleanupUploadedObject(ctx context.Context, objectName string) {
	if objectName == "" {
		return
	}
	if useMinio && minioClient != nil {
		if err := minioClient.RemoveObject(ctx, minioBucket, objectName, minio.RemoveObjectOptions{}); err != nil {
			logger.Warn("清理孤儿MinIO对象失败", "error", err, "object", objectName)
		}
		return
	}
	fullPath := filepath.Join(storageDir, filepath.FromSlash(objectName))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		logger.Warn("清理孤儿本地文件失败", "error", err, "path", fullPath)
	}
}

// ServeLocalFile exposes local-storage objects under /files for inline preview.
// The path is cleaned and checked before joining with storageDir, so a crafted
// URL cannot read arbitrary files from the host.
func (h *FileHandler) ServeLocalFile(ctx context.Context, c *app.RequestContext) {
	// Public inline preview for local images/audio. The same path guard used by
	// DownloadFile keeps requests inside storageDir.
	relativePath := c.Param("filepath")
	relativePath = strings.TrimPrefix(relativePath, "/")
	cleanPath := filepath.Clean(filepath.FromSlash(relativePath))
	if cleanPath == "." || strings.HasPrefix(cleanPath, ".."+string(os.PathSeparator)) || filepath.IsAbs(cleanPath) {
		response.BadRequest(c, "无效的文件路径")
		return
	}

	fullPath := filepath.Join(storageDir, cleanPath)
	if _, err := os.Stat(fullPath); err != nil {
		response.Error(c, "文件不存在或已被删除")
		return
	}
	c.File(fullPath)
}

// DeleteFile deletes a file metadata record and, when supported, the stored
// object. file-service verifies that the operator owns or may delete the file.
func (h *FileHandler) DeleteFile(ctx context.Context, c *app.RequestContext) {
	type deleteReq struct {
		FileID string `json:"file_id"`
	}
	var req deleteReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.FileClient.DeleteFile(ctx, client.NewDeleteFileReq(req.FileID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListFiles returns the current user's files with optional type filtering and
// pagination.
func (h *FileHandler) ListFiles(ctx context.Context, c *app.RequestContext) {
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	fileType := c.DefaultQuery("file_type", "")
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	resp, err := client.FileClient.ListFiles(ctx, client.NewListFilesReq(id, fileType, limit, offset))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
