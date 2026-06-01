package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGLMEmbeddingProviderUsesConfiguredEndpoint(t *testing.T) {
	var gotAuth string
	var gotModel string
	var gotInput string
	var gotDimensions int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			Model      string `json:"model"`
			Input      string `json:"input"`
			Dimensions int    `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = req.Model
		gotInput = req.Input
		gotDimensions = req.Dimensions
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer server.Close()

	provider := NewGLMEmbeddingProvider(server.URL, "secret-token", "embedding-3", 3)
	vector, err := provider.Embed(context.Background(), "你好")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotModel != "embedding-3" || gotInput != "你好" || gotDimensions != 3 {
		t.Fatalf("request = model:%q input:%q dimensions:%d", gotModel, gotInput, gotDimensions)
	}
	if len(vector) != 3 || vector[0] != 0.1 || vector[2] != 0.3 {
		t.Fatalf("vector = %#v", vector)
	}
}
