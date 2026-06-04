package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/file"
	"ClaranAIM/pkg/config"
	"ClaranAIM/pkg/documentparser"
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
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// minioClient 是网关上传/下载二进制对象时使用的 MinIO 客户端。
// 元数据仍由 file-service 管理，因此这里不保存文件归属信息。
var minioClient *minio.Client

// minioBucket 保存对象所在桶名。
var minioBucket string

// useMinio 控制当前进程走 MinIO 还是本地 storageDir。
var useMinio bool

// minioEndpoint 用于把 MinIO 文件 URL 反解回 objectName。
var minioEndpoint string

// storageDir 是本地文件存储根目录，所有本地读取都必须限制在该目录下。
var storageDir string
var fileOCRProvider documentparser.OCRProvider

// InitFileStorage 初始化 API 网关使用的二进制对象存储。
//
// 文件元数据仍由 file-service 持久化；网关只负责浏览器 multipart 上传、
// 下载和预览的字节流边界，因此这里同时支持 MinIO 和本地磁盘两种存储。
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

func InitFileOCR(provider documentparser.OCRProvider) {
	fileOCRProvider = provider
}

// FileHandler 处理浏览器文件上传、预览、下载和文件列表接口。
//
// API 网关负责 HTTP multipart 解析和字节流传输，file-service 负责保存文件元数据
// 以及与权限相关的归属字段，二者职责不要混在一起。
type FileHandler struct{}

// NewFileHandler 创建路由层使用的无状态文件 HTTP handler。
func NewFileHandler() *FileHandler {
	return &FileHandler{}
}

// UploadFile 先保存二进制文件，再请求 file-service 持久化元数据。
//
// 返回给前端的 file_id 必须以 file-service 为准，因为下载、预览和消息媒体载荷
// 后续都会通过这个 ID 查询文件记录。
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

// GetFile 返回单个已上传文件的元数据，不传输文件内容。
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

// DownloadFile 以附件形式向浏览器传输文件。
func (h *FileHandler) DownloadFile(ctx context.Context, c *app.RequestContext) {
	h.serveFileByID(ctx, c, true)
}

// PreviewFile 以内联形式传输文件，用于聊天图片、音频等预览场景。
func (h *FileHandler) PreviewFile(ctx context.Context, c *app.RequestContext) {
	h.serveFileByID(ctx, c, false)
}

// AnalyzeImage 对聊天图片做一次 OCR / 版面解析，供前端和 Agent 工具解释截图使用。
func (h *FileHandler) AnalyzeImage(ctx context.Context, c *app.RequestContext) {
	if fileOCRProvider == nil {
		response.BadRequest(c, "图片OCR未启用：api-gateway 未初始化 OCR provider。请检查 config/api-gateway.yaml 的 document.ocr_provider/ocr_url/ocr_api_key/ocr_model，或先在设置页配置可用 OCR 模型并重启 api-gateway。")
		return
	}
	fileID := c.Param("id")
	resp, err := client.FileClient.GetFile(ctx, client.NewGetFileReq(fileID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if resp == nil || !resp.Success {
		msg := "文件不存在或无权访问"
		if resp != nil && resp.Msg != "" {
			msg = resp.Msg
		}
		response.Error(c, msg)
		return
	}
	if !strings.HasPrefix(strings.ToLower(resp.ContentType), "image/") && !isImageFileName(resp.FileName) {
		response.BadRequest(c, "当前文件不是图片，无法执行图片OCR")
		return
	}
	data, err := readStoredFileBytes(ctx, resp)
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	text, err := fileOCRProvider.ExtractText(ctx, resp.FileName, resp.ContentType, data)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{
		"success":      true,
		"file_id":      fileID,
		"file_name":    resp.FileName,
		"content_type": resp.ContentType,
		"text":         text,
		"msg":          "ok",
	})
}

// serveFileByID 根据 file-service 元数据读取对象并写回浏览器。
// attachment=true 时走下载附件；false 时以内联方式服务图片、语音等预览。
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

	// 本地文件 URL 按 /files/<type>/<object> 存储；拼接 storageDir 前必须清洗路径，
	// 防止构造出的恶意 URL 通过 ../ 逃逸到存储根目录之外。
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

func readStoredFileBytes(ctx context.Context, meta *file.GetFileResp) ([]byte, error) {
	if meta == nil {
		return nil, errors.New("文件不存在")
	}
	if strings.HasPrefix(meta.FileUrl, "http://") || strings.HasPrefix(meta.FileUrl, "https://") {
		if useMinio && minioClient != nil {
			objectName, err := minioObjectNameFromURL(meta.FileUrl)
			if err != nil {
				return nil, err
			}
			obj, err := minioClient.GetObject(ctx, minioBucket, objectName, minio.GetObjectOptions{})
			if err != nil {
				return nil, errors.New("文件读取失败")
			}
			defer obj.Close()
			return io.ReadAll(obj)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.FileUrl, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, errors.New("远程文件读取失败")
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("远程文件返回状态码%d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	if !strings.HasPrefix(meta.FileUrl, "/files/") {
		return nil, errors.New("不支持的文件地址")
	}
	relativePath := strings.TrimPrefix(meta.FileUrl, "/files/")
	cleanPath := filepath.Clean(filepath.FromSlash(relativePath))
	if cleanPath == "." || strings.HasPrefix(cleanPath, ".."+string(os.PathSeparator)) || filepath.IsAbs(cleanPath) {
		return nil, errors.New("无效的文件路径")
	}
	fullPath := filepath.Join(storageDir, cleanPath)
	return os.ReadFile(fullPath)
}

func isImageFileName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

// minioObjectNameFromURL 将保存的 MinIO HTTP URL 反解为桶内 objectName。
// 只接受当前配置 endpoint/bucket 下的 URL，避免通过外部 URL 读取任意对象。
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

// buildUploadObjectName 生成对象存储路径，格式为 <file_type>/<uuid><ext>。
// file_type 支持简单分层，但拒绝绝对路径、空路径和 ..，防止路径穿越。
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

// cleanupUploadedObject 在元数据写入失败时清理已上传的孤儿对象。
// 清理失败只记录告警，不覆盖原始业务错误。
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

// ServeLocalFile 将本地存储对象通过 /files 暴露给浏览器做内联预览。
//
// 请求路径会先经过清洗和越界检查，再与 storageDir 拼接，避免读取主机上的任意文件。
func (h *FileHandler) ServeLocalFile(ctx context.Context, c *app.RequestContext) {
	// 本接口用于本地图片、音频等公开预览；路径防护规则与 DownloadFile 保持一致，
	// 确保请求始终被限制在 storageDir 目录内。
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

// DeleteFile 删除文件元数据，并在存储后端支持时同步删除实际对象。
//
// 操作者是否拥有该文件或是否具备删除权限，由 file-service 统一校验。
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

// ListFiles 按可选文件类型和分页参数返回当前用户的文件列表。
func (h *FileHandler) ListFiles(ctx context.Context, c *app.RequestContext) {
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	fileType := c.DefaultQuery("file_type", "")
	limit, ok := parsePositiveLimit(c, "limit", 20, 100)
	if !ok {
		return
	}
	offset, ok := parseNonNegativeQueryInt64(c, "offset", 0)
	if !ok {
		return
	}
	resp, err := client.FileClient.ListFiles(ctx, client.NewListFilesReq(id, fileType, limit, offset))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
