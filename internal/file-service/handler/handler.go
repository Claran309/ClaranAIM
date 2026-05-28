// Package handler 实现 file-service 的 Kitex RPC 入口。
// Handler 负责把生成的 Thrift 请求转换为 service 调用，并在 RPC 边界处理 nil 和必填字段校验。
package handler

import (
	"ClaranAIM/internal/file-service/service"
	"ClaranAIM/kitex_gen/file"
	"context"
	"errors"
)

// FileServiceImpl 是 file-service 的 Kitex 服务端实现。
type FileServiceImpl struct {
	svc service.FileService
}

// NewFileServiceImpl 将业务服务注入生成的 RPC 服务端。
func NewFileServiceImpl(svc service.FileService) file.FileService {
	return &FileServiceImpl{svc: svc}
}

// UploadFile 为网关已上传的文件保存元数据记录。
func (h *FileServiceImpl) UploadFile(ctx context.Context, req *file.UploadFileReq) (resp *file.UploadFileResp, err error) {
	if req == nil {
		return &file.UploadFileResp{Success: false, Msg: "upload request is nil"}, nil
	}
	if h.svc == nil {
		return &file.UploadFileResp{Success: false, Msg: "file service is not initialized"}, nil
	}
	if req.FileName == "" {
		return &file.UploadFileResp{Success: false, Msg: "file name is required"}, nil
	}
	if req.FileUrl == "" {
		return &file.UploadFileResp{Success: false, Msg: "file url is required"}, nil
	}
	record, err := h.svc.SaveFileRecord(ctx, req.FileName, req.FileType, req.FileSize, req.ContentType, req.FileUrl, req.UploaderId)
	if err != nil {
		return &file.UploadFileResp{Success: false, Msg: err.Error()}, nil
	}
	return &file.UploadFileResp{
		Success: true,
		FileUrl: record.FileURL,
		FileId:  record.FileID,
		Msg:     "保存成功",
	}, nil
}

// GetFile 根据 file_id 返回文件元数据。
func (h *FileServiceImpl) GetFile(ctx context.Context, req *file.GetFileReq) (resp *file.GetFileResp, err error) {
	if req == nil {
		return &file.GetFileResp{Success: false, Msg: "get file request is nil"}, nil
	}
	if h.svc == nil {
		return &file.GetFileResp{Success: false, Msg: "file service is not initialized"}, nil
	}
	record, err := h.svc.GetFile(ctx, req.FileId)
	if err != nil {
		return &file.GetFileResp{Success: false, Msg: err.Error()}, nil
	}
	return &file.GetFileResp{
		Success:     true,
		FileUrl:     record.FileURL,
		FileName:    record.FileName,
		FileType:    record.FileType,
		FileSize:    record.FileSize,
		ContentType: record.ContentType,
		Msg:         "获取成功",
	}, nil
}

// DeleteFile 在 service 层完成所有权校验后删除文件。
func (h *FileServiceImpl) DeleteFile(ctx context.Context, req *file.DeleteFileReq) (resp *file.DeleteFileResp, err error) {
	if req == nil {
		return &file.DeleteFileResp{Success: false, Msg: "delete file request is nil"}, nil
	}
	if h.svc == nil {
		return &file.DeleteFileResp{Success: false, Msg: "file service is not initialized"}, nil
	}
	if req.FileId == "" {
		return &file.DeleteFileResp{Success: false, Msg: errors.New("file id is required").Error()}, nil
	}
	err = h.svc.DeleteFile(ctx, req.FileId, req.OperatorId)
	if err != nil {
		return &file.DeleteFileResp{Success: false, Msg: err.Error()}, nil
	}
	return &file.DeleteFileResp{Success: true, Msg: "删除成功"}, nil
}

// ListFiles 返回某个上传者的分页文件元数据列表。
func (h *FileServiceImpl) ListFiles(ctx context.Context, req *file.ListFilesReq) (resp *file.ListFilesResp, err error) {
	if req == nil {
		return &file.ListFilesResp{Success: false, Msg: "list files request is nil"}, nil
	}
	if h.svc == nil {
		return &file.ListFilesResp{Success: false, Msg: "file service is not initialized"}, nil
	}
	limit := req.Limit
	offset := req.Offset
	if limit <= 0 {
		limit = 20
	}

	files, total, err := h.svc.ListFiles(ctx, req.UploaderId, req.FileType, limit, offset)
	if err != nil {
		return &file.ListFilesResp{Success: false, Msg: err.Error()}, nil
	}

	var list []*file.FileInfo
	for _, f := range files {
		list = append(list, &file.FileInfo{
			FileId:      f.FileID,
			FileName:    f.FileName,
			FileType:    f.FileType,
			FileSize:    f.FileSize,
			ContentType: f.ContentType,
			FileUrl:     f.FileURL,
			UploaderId:  f.UploaderID,
			CreatedAt:   f.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &file.ListFilesResp{Success: true, Files: list, Total: total, Msg: "获取成功"}, nil
}
