package apigateway_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIGatewayDoesNotImportServiceInternals(t *testing.T) {
	root := filepath.Join("..", "..")
	prefix := `"ClaranAIM/internal/`
	forbidden := []string{
		prefix + "memory-service/",
		prefix + "msg-core-service/",
		prefix + "settings-service/",
		prefix + "agent-manager-service/",
		prefix + "agent-runtime-service/",
		prefix + "user-service/",
		prefix + "group-service/",
		prefix + "file-service/",
		prefix + "msg-history-service/",
	}
	err := filepath.WalkDir(filepath.Join(root, "internal", "api-gateway"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		for _, pattern := range forbidden {
			if strings.Contains(text, pattern) {
				t.Fatalf("api-gateway must not import service internals: %s contains %s", path, pattern)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAPIGatewayRouterDoesNotExposeLegacyBotRoutes(t *testing.T) {
	root := filepath.Join("..", "..")
	routerPath := filepath.Join(root, "internal", "api-gateway", "router", "router.go")
	content, err := os.ReadFile(routerPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "\"/bot") {
		t.Fatalf("api-gateway router must expose Agent routes only, found legacy /bot route in %s", routerPath)
	}
}
