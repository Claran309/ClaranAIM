package internal_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMicroservicesDoNotImportOtherServiceInternals(t *testing.T) {
	root := filepath.Join("..")
	services := []string{
		"api-gateway",
		"agent-manager-service",
		"agent-runtime-service",
		"file-service",
		"group-service",
		"memory-service",
		"msg-core-service",
		"msg-history-service",
		"settings-service",
		"user-service",
		"websocket-gateway",
	}
	for _, service := range services {
		service := service
		t.Run(service, func(t *testing.T) {
			serviceRoot := filepath.Join(root, service)
			if _, err := os.Stat(serviceRoot); err != nil {
				return
			}
			err := filepath.WalkDir(serviceRoot, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
					return err
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				text := string(content)
				for _, other := range services {
					if other == service {
						continue
					}
					pattern := `"ClaranAIM/internal/` + other + `/`
					if strings.Contains(text, pattern) {
						t.Fatalf("%s must not import another service internal package %s", path, pattern)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
