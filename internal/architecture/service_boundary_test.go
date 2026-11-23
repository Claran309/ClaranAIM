// Package architecture_test 把服务边界约束固化成可执行的测试。
//
// 规则：internal/<service> 只能依赖自己的子包和 ClaranAIM/pkg、ClaranAIM/kitex_gen，
// 不允许直接 import 另一个服务内部的 Go 包。服务之间只能通过 RPC / 事件 / HTTP 契约交互。
package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到 go.mod，无法定位模块根目录")
		}
		dir = parent
	}
}

func TestNoCrossServiceInternalImports(t *testing.T) {
	root := moduleRoot(t)
	internalDir := filepath.Join(root, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		t.Fatalf("ReadDir(internal) error = %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		service := entry.Name()
		serviceDir := filepath.Join(internalDir, service)
		err := filepath.WalkDir(serviceDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return parseErr
			}
			for _, imp := range file.Imports {
				importPath, unquoteErr := strconv.Unquote(imp.Path.Value)
				if unquoteErr != nil {
					continue
				}
				const prefix = "ClaranAIM/internal/"
				if !strings.HasPrefix(importPath, prefix) {
					continue
				}
				importedService := strings.Split(strings.TrimPrefix(importPath, prefix), "/")[0]
				if importedService == service {
					continue
				}
				t.Errorf("%s 跨服务 import 了 %s；服务之间应通过 RPC / 事件 / HTTP 契约交互",
					filepath.ToSlash(path), importPath)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir(%s) error = %v", serviceDir, err)
		}
	}
}

func TestAgentKitDoesNotImportServices(t *testing.T) {
	root := moduleRoot(t)
	agentkitDir := filepath.Join(root, "pkg", "agentkit")
	err := filepath.WalkDir(agentkitDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			importPath, unquoteErr := strconv.Unquote(imp.Path.Value)
			if unquoteErr != nil {
				continue
			}
			if strings.HasPrefix(importPath, "ClaranAIM/internal/") {
				t.Errorf("%s 依赖了服务内部包 %s；pkg/agentkit 必须保持可复用",
					filepath.ToSlash(path), importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(pkg/agentkit) error = %v", err)
	}
}
