package service

import (
	"ClaranAIM/internal/file-service/model"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fileRepoStub struct {
	record  *model.FileRecord
	deleted string
}

func (r *fileRepoStub) CreateFile(ctx context.Context, file *model.FileRecord) error {
	r.record = file
	return nil
}

func (r *fileRepoStub) GetFileByID(ctx context.Context, fileID string) (*model.FileRecord, error) {
	if r.record == nil || r.record.FileID != fileID {
		return nil, nil
	}
	return r.record, nil
}

func (r *fileRepoStub) DeleteFile(ctx context.Context, fileID string) error {
	r.deleted = fileID
	return nil
}

func (r *fileRepoStub) ListFiles(ctx context.Context, uploaderID int64, fileType string, limit, offset int64) ([]model.FileRecord, int64, error) {
	return nil, 0, nil
}

func TestDeleteFileRemovesLocalObjectFromStoredFileURL(t *testing.T) {
	storageDir := t.TempDir()
	objectPath := filepath.Join(storageDir, "image", "object-from-gateway.png")
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatalf("mkdir object dir: %v", err)
	}
	if err := os.WriteFile(objectPath, []byte("image"), 0o644); err != nil {
		t.Fatalf("write object: %v", err)
	}

	repo := &fileRepoStub{
		record: &model.FileRecord{
			FileID:     "metadata-id-from-file-service",
			FileName:   "avatar.png",
			FileType:   "image",
			FileURL:    "/files/image/object-from-gateway.png",
			UploaderID: 1000000001,
		},
	}
	svc := NewFileService(repo, storageDir, "", "", "", "", false)

	if err := svc.DeleteFile(context.Background(), "metadata-id-from-file-service", 1000000001); err != nil {
		t.Fatalf("DeleteFile returned error: %v", err)
	}

	if _, err := os.Stat(objectPath); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists or stat failed unexpectedly: %v", err)
	}
	if repo.deleted != "metadata-id-from-file-service" {
		t.Fatalf("repo.deleted = %q, want metadata-id-from-file-service", repo.deleted)
	}
}

func TestUploadFileRejectsTraversalFileType(t *testing.T) {
	storageDir := t.TempDir()
	svc := NewFileService(&fileRepoStub{}, storageDir, "", "", "", "", false)

	_, err := svc.UploadFile(context.Background(), "avatar.png", "../avatar", 4, "image/png", 1000000001, strings.NewReader("data"))
	if err == nil {
		t.Fatal("UploadFile should reject path traversal file type")
	}
}
