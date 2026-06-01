// Package documentparser 提供知识库上传文件的轻量文本抽取能力。
package documentparser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const maxParsedTextRunes = 2_000_000

// OCRProvider 抽象外部 OCR / 文档版面解析能力。
type OCRProvider interface {
	ExtractText(ctx context.Context, filename, contentType string, data []byte) (string, error)
}

// ParseOptions 保存文档解析的可选外部能力。
type ParseOptions struct {
	OCR OCRProvider
}

// ParsedDocument 是文件解析后的可入库文本。
type ParsedDocument struct {
	Title      string
	Content    string
	SourceType string
}

// Parse 根据文件名、Content-Type 和二进制内容抽取可写入 RAG 的正文。
// 文本和代码文件按 UTF-8 原文保留，交给 rag-service 按 Markdown 标题、代码结构或段落层级继续切分。
// PDF 使用轻量文本流抽取兜底，复杂扫描版 PDF 后续应接专业解析器。
func Parse(filename, contentType string, data []byte) (ParsedDocument, error) {
	return ParseWithOptions(context.Background(), filename, contentType, data, ParseOptions{})
}

// ParseWithOptions 根据文件名、Content-Type 和二进制内容抽取可写入 RAG 的正文。
// 当 PDF 本地未解析到文本，或上传的是图片文件时，会使用 OCRProvider 兜底。
func ParseWithOptions(ctx context.Context, filename, contentType string, data []byte, opts ParseOptions) (ParsedDocument, error) {
	if len(data) == 0 {
		return ParsedDocument{}, errors.New("文件内容为空")
	}
	sourceType := inferSourceType(filename, contentType)
	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if title == "" || title == "." {
		title = "未命名文档"
	}
	var content string
	var err error
	switch sourceType {
	case "txt", "md", "markdown", "text":
		content, err = parsePlainText(data)
	case "go", "js", "ts", "tsx", "jsx", "py", "java", "c", "cpp", "cc", "cxx", "h", "hpp", "rs", "sql", "json", "yaml", "yml", "toml", "xml", "html", "css", "scss", "sh", "bat", "ps1":
		content, err = parsePlainText(data)
	case "pdf":
		content, err = parsePDFText(data)
		if err != nil && opts.OCR != nil {
			content, err = opts.OCR.ExtractText(ctx, filename, contentType, data)
		}
	case "docx":
		content, err = parseDocxText(data)
	case "png", "jpg", "jpeg", "webp", "bmp", "gif", "tif", "tiff":
		if opts.OCR == nil {
			return ParsedDocument{}, errors.New("图片文件需要配置OCR服务后才能解析")
		}
		content, err = opts.OCR.ExtractText(ctx, filename, contentType, data)
	default:
		return ParsedDocument{}, fmt.Errorf("暂不支持的知识库文件类型: %s", sourceType)
	}
	if err != nil {
		return ParsedDocument{}, err
	}
	content = normalizeParsedText(content)
	if content == "" {
		return ParsedDocument{}, errors.New("未能从文件中解析出有效文本")
	}
	return ParsedDocument{Title: title, Content: content, SourceType: sourceType}, nil
}

// GLMLayoutOCRProvider 调用智谱 layout_parsing / glm-ocr 接口。
type GLMLayoutOCRProvider struct {
	URL    string
	APIKey string
	Model  string
	Client *http.Client
}

func NewGLMLayoutOCRProvider(url, apiKey, model string) *GLMLayoutOCRProvider {
	return &GLMLayoutOCRProvider{
		URL:    strings.TrimSpace(url),
		APIKey: strings.TrimSpace(apiKey),
		Model:  defaultString(model, "glm-ocr"),
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *GLMLayoutOCRProvider) ExtractText(ctx context.Context, filename, contentType string, data []byte) (string, error) {
	if p == nil || p.URL == "" || p.APIKey == "" {
		return "", errors.New("OCR服务未配置")
	}
	fileValue := buildOCRDataURL(filename, contentType, data)
	payload, err := json.Marshal(map[string]interface{}{
		"model": p.Model,
		"file":  fileValue,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OCR接口返回状态码%d", resp.StatusCode)
	}
	var decoded interface{}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	text := extractOCRText(decoded)
	if strings.TrimSpace(text) == "" {
		return "", errors.New("OCR接口未返回有效文本")
	}
	return text, nil
}

func inferSourceType(filename, contentType string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if ext != "" {
		if ext == "mdx" {
			return "markdown"
		}
		return ext
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch strings.ToLower(mediaType) {
	case "text/plain":
		return "txt"
	case "text/markdown":
		return "md"
	case "text/x-go":
		return "go"
	case "application/json":
		return "json"
	case "application/x-yaml", "text/yaml":
		return "yaml"
	case "application/pdf":
		return "pdf"
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	default:
		if strings.HasPrefix(mediaType, "text/") {
			return "text"
		}
	}
	return "unknown"
}

func buildOCRDataURL(filename, contentType string, data []byte) string {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "" {
		mediaType = contentTypeByExt(filename)
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func contentTypeByExt(filename string) string {
	switch strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".") {
	case "pdf":
		return "application/pdf"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	case "gif":
		return "image/gif"
	case "tif", "tiff":
		return "image/tiff"
	default:
		return ""
	}
}

func extractOCRText(decoded interface{}) string {
	var parts []string
	collectOCRText(decoded, &parts)
	return strings.Join(parts, "\n")
}

func collectOCRText(value interface{}, parts *[]string) {
	switch v := value.(type) {
	case map[string]interface{}:
		for _, key := range []string{"md_results", "markdown", "text", "content"} {
			if raw, ok := v[key]; ok {
				collectOCRText(raw, parts)
			}
		}
		if data, ok := v["data"]; ok {
			collectOCRText(data, parts)
		}
	case []interface{}:
		for _, item := range v {
			collectOCRText(item, parts)
		}
	case string:
		text := strings.TrimSpace(v)
		if text != "" {
			*parts = append(*parts, text)
		}
	}
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parsePlainText(data []byte) (string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) {
		return "", errors.New("文本文件不是有效UTF-8编码")
	}
	return string(data), nil
}

var (
	pdfLiteralRe = regexp.MustCompile(`\((?:\\.|[^\\)])*\)\s*T[jJ]`)
	pdfArrayRe   = regexp.MustCompile(`\[((?:\s*\((?:\\.|[^\\)])*\)\s*)+)\]\s*TJ`)
)

func parsePDFText(data []byte) (string, error) {
	raw := string(data)
	var parts []string
	for _, match := range pdfLiteralRe.FindAllString(raw, -1) {
		start := strings.Index(match, "(")
		end := strings.LastIndex(match, ")")
		if start >= 0 && end > start {
			parts = append(parts, decodePDFLiteral(match[start+1:end]))
		}
	}
	for _, match := range pdfArrayRe.FindAllString(raw, -1) {
		for _, item := range regexp.MustCompile(`\((?:\\.|[^\\)])*\)`).FindAllString(match, -1) {
			if len(item) >= 2 {
				parts = append(parts, decodePDFLiteral(item[1:len(item)-1]))
			}
		}
	}
	if len(parts) == 0 {
		return "", errors.New("当前PDF未解析出文本；如果是扫描件，请先OCR或转换为可复制文本PDF")
	}
	return strings.Join(parts, "\n"), nil
}

func decodePDFLiteral(text string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\(`, `(`,
		`\)`, `)`,
		`\n`, "\n",
		`\r`, "\n",
		`\t`, "\t",
		`\b`, "",
		`\f`, "\n",
	)
	return replacer.Replace(text)
}

func parseDocxText(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("DOCX文件结构无效")
	}
	var documentXML []byte
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", errors.New("读取DOCX正文失败")
		}
		documentXML, err = io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return "", errors.New("读取DOCX正文失败")
		}
		break
	}
	if len(documentXML) == 0 {
		return "", errors.New("DOCX文件缺少正文内容")
	}
	decoder := xml.NewDecoder(bytes.NewReader(documentXML))
	var parts []string
	var current strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.New("解析DOCX正文XML失败")
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" && current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		case xml.CharData:
			if len(t) > 0 {
				current.WriteString(string(t))
			}
		case xml.EndElement:
			if t.Name.Local == "p" && current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if text == "" {
		return "", errors.New("未能从DOCX文件中解析出有效文本")
	}
	return text, nil
}

func normalizeParsedText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if !blank {
				out = append(out, "")
				blank = true
			}
			continue
		}
		blank = false
		out = append(out, line)
	}
	normalized := strings.TrimSpace(strings.Join(out, "\n"))
	runes := []rune(normalized)
	if len(runes) > maxParsedTextRunes {
		normalized = string(runes[:maxParsedTextRunes])
	}
	return normalized
}
