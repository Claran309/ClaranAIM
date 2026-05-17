package handler

import "testing"

func TestBuildUploadObjectNameRejectsPathTraversalFileType(t *testing.T) {
	if _, err := buildUploadObjectName("../avatar", "file-id", ".png"); err == nil {
		t.Fatal("path traversal file_type should be rejected")
	}
}

func TestBuildUploadObjectNameAllowsSimpleNestedType(t *testing.T) {
	got, err := buildUploadObjectName("image/avatar", "file-id", ".png")
	if err != nil {
		t.Fatalf("buildUploadObjectName returned error: %v", err)
	}
	if got != "image/avatar/file-id.png" {
		t.Fatalf("objectName = %q, want image/avatar/file-id.png", got)
	}
}
