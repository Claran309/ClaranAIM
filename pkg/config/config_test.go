package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDoesNotLeakValuesBetweenConfigFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")

	if err := os.WriteFile(first, []byte(`
service:
  name: first-service
  address: "127.0.0.1:1"
kafka:
  enabled: true
  client_id: first-client
governance:
  rpc:
    timeout_ms: 1234
`), 0o644); err != nil {
		t.Fatalf("write first config: %v", err)
	}
	if err := os.WriteFile(second, []byte(`
service:
  name: second-service
  address: "127.0.0.1:2"
`), 0o644); err != nil {
		t.Fatalf("write second config: %v", err)
	}

	firstCfg, err := Load(first)
	if err != nil {
		t.Fatalf("Load(first) returned error: %v", err)
	}
	if firstCfg.Kafka.ClientID != "first-client" {
		t.Fatalf("first Kafka client id = %q, want first-client", firstCfg.Kafka.ClientID)
	}

	secondCfg, err := Load(second)
	if err != nil {
		t.Fatalf("Load(second) returned error: %v", err)
	}
	if secondCfg.Service.Name != "second-service" {
		t.Fatalf("second service name = %q, want second-service", secondCfg.Service.Name)
	}
	if secondCfg.Kafka.ClientID != "second-service" {
		t.Fatalf("second Kafka client id = %q, want second-service", secondCfg.Kafka.ClientID)
	}
	if secondCfg.Governance.RPC.TimeoutMS != 60000 {
		t.Fatalf("second RPC timeout = %d, want default 60000", secondCfg.Governance.RPC.TimeoutMS)
	}
	if secondCfg.Governance.AgentRPC.TimeoutMS != 0 {
		t.Fatalf("second Agent RPC timeout = %d, want disabled timeout 0", secondCfg.Governance.AgentRPC.TimeoutMS)
	}
	if !secondCfg.Governance.AgentRPC.CircuitBreaker {
		t.Fatal("second Agent RPC circuit breaker = false, want default true")
	}
	if secondCfg.Governance.AgentRPC.MaxQPS != 500 {
		t.Fatalf("second Agent RPC max qps = %d, want 500", secondCfg.Governance.AgentRPC.MaxQPS)
	}
	if !secondCfg.DTM.Enabled {
		t.Fatal("second DTM enabled = false, want default true")
	}
	if secondCfg.DTM.Server != "http://localhost:36789" {
		t.Fatalf("second DTM server = %q, want http://localhost:36789", secondCfg.DTM.Server)
	}
	if secondCfg.DTM.GroupServiceURL != "http://127.0.0.1:9102" {
		t.Fatalf("second DTM group service url = %q, want http://127.0.0.1:9102", secondCfg.DTM.GroupServiceURL)
	}
	if secondCfg.DTM.MsgCoreServiceURL != "http://127.0.0.1:9103" {
		t.Fatalf("second DTM msg-core service url = %q, want http://127.0.0.1:9103", secondCfg.DTM.MsgCoreServiceURL)
	}
}

func TestLoadSettingsSecretKeyFromEnvironment(t *testing.T) {
	t.Setenv("SETTINGS_SECRET_KEY", "settings-secret-for-test")
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(path, []byte(`
service:
  name: settings-service
  address: "127.0.0.1:9009"
`), 0o644); err != nil {
		t.Fatalf("write settings config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Settings.SecretKey != "settings-secret-for-test" {
		t.Fatalf("settings secret key = %q", cfg.Settings.SecretKey)
	}
}

func TestLoadObservabilityDefaultsAndEnvOverrides(t *testing.T) {
	t.Setenv("OBSERVABILITY_ENABLED", "true")
	t.Setenv("OBSERVABILITY_METRICS_ADDRESS", "127.0.0.1:19999")
	t.Setenv("OBSERVABILITY_GRAFANA_URL", "http://grafana.local")
	dir := t.TempDir()
	path := filepath.Join(dir, "observability.yaml")
	if err := os.WriteFile(path, []byte(`
service:
  name: api-gateway
  address: "127.0.0.1:8080"
observability:
  environment: "test"
  otlp_endpoint: "http://127.0.0.1:4318"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !cfg.Observability.Enabled {
		t.Fatal("observability enabled = false, want true")
	}
	if cfg.Observability.Environment != "test" {
		t.Fatalf("environment=%q, want test", cfg.Observability.Environment)
	}
	if cfg.Observability.OTLPEndpoint != "http://127.0.0.1:4318" {
		t.Fatalf("otlp endpoint=%q", cfg.Observability.OTLPEndpoint)
	}
	if cfg.Observability.MetricsAddress != "127.0.0.1:19999" {
		t.Fatalf("metrics address=%q", cfg.Observability.MetricsAddress)
	}
	if cfg.Observability.GrafanaURL != "http://grafana.local" {
		t.Fatalf("grafana url=%q", cfg.Observability.GrafanaURL)
	}
	if cfg.Observability.JaegerURL == "" || cfg.Observability.KibanaURL == "" || cfg.Observability.PrometheusURL == "" {
		t.Fatalf("observability panel defaults missing: %#v", cfg.Observability)
	}
}

func TestLoadNeo4jDefaultsAndEnvOverrides(t *testing.T) {
	t.Setenv("NEO4J_ENABLED", "false")
	t.Setenv("NEO4J_URI", "bolt://neo4j.internal:7687")
	t.Setenv("NEO4J_USERNAME", "graph-user")
	t.Setenv("NEO4J_PASSWORD", "graph-pass")
	t.Setenv("NEO4J_DATABASE", "graphdb")
	dir := t.TempDir()
	path := filepath.Join(dir, "rag.yaml")
	if err := os.WriteFile(path, []byte(`
service:
  name: rag-service
  address: "127.0.0.1:9112"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Neo4j.Enabled {
		t.Fatal("neo4j enabled = true, want false from env")
	}
	if cfg.Neo4j.URI != "bolt://neo4j.internal:7687" || cfg.Neo4j.Username != "graph-user" || cfg.Neo4j.Password != "graph-pass" || cfg.Neo4j.Database != "graphdb" {
		t.Fatalf("neo4j config = %#v", cfg.Neo4j)
	}
}
