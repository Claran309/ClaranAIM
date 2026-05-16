package handler

import (
	"ClaranAIM/kitex_gen/file"
	"context"
	"testing"
)

func TestUploadFileRejectsNilRequestWithoutPanic(t *testing.T) {
	h := &FileServiceImpl{}

	resp, err := h.UploadFile(context.Background(), nil)
	if err != nil {
		t.Fatalf("UploadFile returned transport error: %v", err)
	}
	if resp == nil {
		t.Fatal("UploadFile returned nil response")
	}
	if resp.Success {
		t.Fatal("UploadFile succeeded for nil request")
	}
}

func TestUploadFileRejectsUninitializedServiceWithoutPanic(t *testing.T) {
	h := &FileServiceImpl{}
	req := &file.UploadFileReq{
		FileName:    "report.pdf",
		FileType:    "file",
		FileSize:    100,
		ContentType: "application/pdf",
		FileUrl:     "http://localhost:9000/uploads/report.pdf",
		UploaderId:  1,
	}

	resp, err := h.UploadFile(context.Background(), req)
	if err != nil {
		t.Fatalf("UploadFile returned transport error: %v", err)
	}
	if resp == nil {
		t.Fatal("UploadFile returned nil response")
	}
	if resp.Success {
		t.Fatal("UploadFile succeeded with an uninitialized service")
	}
}
