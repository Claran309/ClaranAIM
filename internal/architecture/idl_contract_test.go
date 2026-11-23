package architecture_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var (
	thriftStructRe  = regexp.MustCompile(`(?m)^struct\s+(\w+)\s*\{`)
	thriftServiceRe = regexp.MustCompile(`(?m)^service\s+(\w+)\s*\{`)
	thriftFieldRe   = regexp.MustCompile(`(?m)^\s*(\d+)\s*:\s*(?:optional\s+)?[\w<>,\s\.]+?\s+(\w+)`)
	thriftMethodRe  = regexp.MustCompile(`^([\w<>,\s\.]+?)\s+(\w+)\s*\(\s*1\s*:`)
)

type thriftBlock struct {
	Name string
	Body string
}

// TestIDLGeneratedCodeIsInSync 检查 IDL 与 kitex_gen 的目录、服务名和结构体是否一致。
func TestIDLGeneratedCodeIsInSync(t *testing.T) {
	root := moduleRoot(t)
	idlDir := filepath.Join(root, "idl")
	entries, err := os.ReadDir(idlDir)
	if err != nil {
		t.Fatalf("ReadDir(idl) error = %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".thrift") {
			continue
		}
		pkg := strings.TrimSuffix(entry.Name(), ".thrift")
		genDir := filepath.Join(root, "kitex_gen", pkg)
		if _, statErr := os.Stat(genDir); statErr != nil {
			t.Errorf("IDL %s 没有对应的 kitex_gen/%s 目录", entry.Name(), pkg)
			continue
		}
		genFile := filepath.Join(genDir, pkg+".go")
		genContent, readErr := os.ReadFile(genFile)
		if readErr != nil {
			t.Errorf("读取生成文件 %s 失败: %v", genFile, readErr)
			continue
		}
		idlContent, readErr := os.ReadFile(filepath.Join(idlDir, entry.Name()))
		if readErr != nil {
			t.Fatalf("读取 IDL %s 失败: %v", entry.Name(), readErr)
		}
		for _, block := range thriftBlocks(string(idlContent), thriftStructRe) {
			if !strings.Contains(string(genContent), "type "+block.Name+" struct") {
				t.Errorf("%s 的 struct %s 未出现在生成代码中，请重新生成 Kitex 代码", entry.Name(), block.Name)
			}
		}
	}
}

// TestThriftStructFieldIDsAreUnique 检查每个 struct 的字段编号不重复，防止序列化错位。
func TestThriftStructFieldIDsAreUnique(t *testing.T) {
	root := moduleRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "idl", "*.thrift"))
	if err != nil {
		t.Fatalf("Glob(idl) error = %v", err)
	}
	for _, file := range files {
		content, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatalf("读取 %s 失败: %v", file, readErr)
		}
		for _, block := range thriftBlocks(string(content), thriftStructRe) {
			ids := make([]int, 0)
			for _, match := range thriftFieldRe.FindAllStringSubmatch(block.Body, -1) {
				id, convErr := strconv.Atoi(match[1])
				if convErr != nil {
					continue
				}
				ids = append(ids, id)
			}
			sort.Ints(ids)
			for i := 1; i < len(ids); i++ {
				if ids[i] == ids[i-1] {
					t.Errorf("%s struct %s 字段编号 %d 重复", filepath.Base(file), block.Name, ids[i])
				}
			}
		}
	}
}

// TestThriftServiceMethodsExistInGeneratedClient 检查 IDL 声明的 RPC 方法都出现在生成客户端接口中。
func TestThriftServiceMethodsExistInGeneratedClient(t *testing.T) {
	root := moduleRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "idl", "*.thrift"))
	if err != nil {
		t.Fatalf("Glob(idl) error = %v", err)
	}
	checked := 0
	for _, file := range files {
		content, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatalf("读取 %s 失败: %v", file, readErr)
		}
		pkg := strings.TrimSuffix(filepath.Base(file), ".thrift")
		pkgDir := filepath.Join(root, "kitex_gen", pkg)
		for _, service := range thriftBlocks(string(content), thriftServiceRe) {
			clientFile := findGeneratedClient(t, pkgDir, service.Name)
			if clientFile == "" {
				t.Errorf("%s 服务 %s 没有找到生成客户端 client.go", filepath.Base(file), service.Name)
				continue
			}
			clientContent, clientErr := os.ReadFile(clientFile)
			if clientErr != nil {
				t.Errorf("读取生成客户端 %s 失败: %v", clientFile, clientErr)
				continue
			}
			for _, line := range strings.Split(service.Body, "\n") {
				match := thriftMethodRe.FindStringSubmatch(strings.TrimSpace(line))
				if match == nil {
					continue
				}
				method := match[2]
				if !strings.Contains(string(clientContent), method+"(ctx context.Context") {
					t.Errorf("%s 服务 %s 的方法 %s 未出现在生成客户端接口中", filepath.Base(file), service.Name, method)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("没有解析到任何 Thrift service 方法，契约测试可能失效")
	}
}

// findGeneratedClient 按 service 名定位 kitex_gen/<pkg>/<serviceDir>/client.go。
// Kitex 生成目录会去掉下划线并转小写（BotRuntimeService -> botruntimeservice），因此不能直接拼目录名。
func findGeneratedClient(t *testing.T, pkgDir, serviceName string) string {
	t.Helper()
	expected := strings.ToLower(strings.TrimSuffix(serviceName, "Service")) + "service"
	exact := filepath.Join(pkgDir, expected, "client.go")
	if _, err := os.Stat(exact); err == nil {
		return exact
	}
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.EqualFold(entry.Name(), expected) {
			continue
		}
		candidate := filepath.Join(pkgDir, entry.Name(), "client.go")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}
	return ""
}

func thriftBlocks(content string, re *regexp.Regexp) []thriftBlock {
	locs := re.FindAllStringSubmatchIndex(content, -1)
	blocks := make([]thriftBlock, 0, len(locs))
	for _, loc := range locs {
		name := content[loc[2]:loc[3]]
		// 正则已经消费了开头的 "{"，body 从匹配结束位置开始。
		start := loc[1]
		depth := 1
		end := start
		for end < len(content) && depth > 0 {
			switch content[end] {
			case '{':
				depth++
			case '}':
				depth--
			}
			end++
		}
		blocks = append(blocks, thriftBlock{Name: name, Body: content[start : end-1]})
	}
	return blocks
}
