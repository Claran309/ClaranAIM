package documentparser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParsePlainTextDocument(t *testing.T) {
	doc, err := Parse("notes.txt", "text/plain; charset=utf-8", []byte("第一行\n第二行"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if doc.SourceType != "txt" {
		t.Fatalf("SourceType = %q, want txt", doc.SourceType)
	}
	if !strings.Contains(doc.Content, "第一行") || !strings.Contains(doc.Content, "第二行") {
		t.Fatalf("Content = %q, want original UTF-8 text", doc.Content)
	}
}

func TestParseGoSourceAsStructuredTextDocument(t *testing.T) {
	source := []byte(`package message

// SendMessageExt 发送消息并写入事件。
func SendMessageExt(content string) error {
	return nil
}
`)
	doc, err := Parse("service.go", "text/x-go", source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if doc.SourceType != "go" {
		t.Fatalf("SourceType = %q, want go", doc.SourceType)
	}
	if !strings.Contains(doc.Content, "func SendMessageExt") || !strings.Contains(doc.Content, "发送消息") {
		t.Fatalf("Content = %q, want Go source text", doc.Content)
	}
}

func TestParsePDFDocumentExtractsLiteralText(t *testing.T) {
	pdf := []byte(`%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 55 >>
stream
BT
/F1 12 Tf
72 720 Td
(Hello PDF Knowledge) Tj
ET
endstream
endobj
trailer
<< /Root 1 0 R >>
%%EOF`)
	doc, err := Parse("manual.pdf", "application/pdf", pdf)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if doc.SourceType != "pdf" {
		t.Fatalf("SourceType = %q, want pdf", doc.SourceType)
	}
	if !strings.Contains(doc.Content, "Hello PDF Knowledge") {
		t.Fatalf("Content = %q, want extracted PDF text", doc.Content)
	}
}

func TestParseWithOptionsFallsBackToOCRWhenPDFHasNoText(t *testing.T) {
	provider := &fakeOCRProvider{text: "OCR 识别出的扫描件正文"}
	doc, err := ParseWithOptions(context.Background(), "scan.pdf", "application/pdf", []byte("%PDF-1.4 no text operators"), ParseOptions{
		OCR: provider,
	})
	if err != nil {
		t.Fatalf("ParseWithOptions returned error: %v", err)
	}
	if !provider.called {
		t.Fatalf("OCR provider was not called")
	}
	if doc.SourceType != "pdf" {
		t.Fatalf("SourceType = %q, want pdf", doc.SourceType)
	}
	if !strings.Contains(doc.Content, "OCR 识别") {
		t.Fatalf("Content = %q, want OCR text", doc.Content)
	}
}

func TestGLMLayoutOCRProviderExtractsMarkdownResults(t *testing.T) {
	var gotModel string
	var gotFile string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want bearer key", r.Header.Get("Authorization"))
		}
		var req struct {
			Model string `json:"model"`
			File  string `json:"file"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		gotModel = req.Model
		gotFile = req.File
		_, _ = w.Write([]byte(`{"data":{"md_results":["# 标题\nOCR正文"]}}`))
	}))
	defer server.Close()

	provider := NewGLMLayoutOCRProvider(server.URL, "test-key", "glm-ocr")
	text, err := provider.ExtractText(context.Background(), "scan.png", "image/png", []byte("fake-image"))
	if err != nil {
		t.Fatalf("ExtractText returned error: %v", err)
	}
	if gotModel != "glm-ocr" {
		t.Fatalf("model = %q, want glm-ocr", gotModel)
	}
	if !strings.HasPrefix(gotFile, "data:image/png;base64,") {
		t.Fatalf("file = %q, want data url", gotFile)
	}
	if !strings.Contains(text, "OCR正文") {
		t.Fatalf("text = %q, want markdown OCR result", text)
	}
}

func TestParseDocxDocumentExtractsDocumentXMLText(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("Create document.xml failed: %v", err)
	}
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>项目知识库</w:t></w:r></w:p>
    <w:p><w:r><w:t>支持 Word 文档解析</w:t></w:r></w:p>
  </w:body>
</w:document>`))
	if err := zw.Close(); err != nil {
		t.Fatalf("Close docx zip failed: %v", err)
	}
	doc, err := Parse("plan.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", buf.Bytes())
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if doc.SourceType != "docx" {
		t.Fatalf("SourceType = %q, want docx", doc.SourceType)
	}
	if !strings.Contains(doc.Content, "项目知识库") || !strings.Contains(doc.Content, "支持 Word 文档解析") {
		t.Fatalf("Content = %q, want extracted docx text", doc.Content)
	}
}

type fakeOCRProvider struct {
	called bool
	text   string
}

func (p *fakeOCRProvider) ExtractText(ctx context.Context, filename, contentType string, data []byte) (string, error) {
	_ = ctx
	_ = filename
	_ = contentType
	_ = data
	p.called = true
	return p.text, nil
}
