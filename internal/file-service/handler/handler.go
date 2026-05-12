package handler

import (
	"ClaranAIM/internal/file-service/service"
	"ClaranAIM/kitex_gen/file"
	"context"
)

type FileServiceImpl struct {
	svc service.FileService
}

func NewFileServiceImpl(svc service.FileService) file.FileService {
	return &FileServiceImpl{svc: svc}
}

func (h *FileServiceImpl) UploadFile(ctx context.Context, req *file.UploadFileReq) (resp *file.UploadFileResp, err error) {
	record, err := h.svc.UploadFile(ctx, req.FileName, req.FileType, req.FileSize, req.ContentType, req.UploaderId, nil)
	if err != nil {
		return &file.UploadFileResp{Success: false, Msg: err.Error()}, nil
	}
	return &file.UploadFileResp{
		Success: true,
		FileUrl: record.FileURL,
		FileId:  record.FileID,
		Msg:     "上传成功",
	}, nil
}

func (h *FileServiceImpl) GetFile(ctx context.Context, req *file.GetFileReq) (resp *file.GetFileResp, err error) {
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

func (h *FileServiceImpl) DeleteFile(ctx context.Context, req *file.DeleteFileReq) (resp *file.DeleteFileResp, err error) {
	err = h.svc.DeleteFile(ctx, req.FileId, req.OperatorId)
	if err != nil {
		return &file.DeleteFileResp{Success: false, Msg: err.Error()}, nil
	}
	return &file.DeleteFileResp{Success: true, Msg: "删除成功"}, nil
}

func (h *FileServiceImpl) ListFiles(ctx context.Context, req *file.ListFilesReq) (resp *file.ListFilesResp, err error) {
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
