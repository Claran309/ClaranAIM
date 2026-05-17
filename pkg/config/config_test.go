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
	if secondCfg.Governance.RPC.TimeoutMS != 5000 {
		t.Fatalf("second RPC timeout = %d, want default 5000", secondCfg.Governance.RPC.TimeoutMS)
	}
	if !secondCfg.DTM.Enabled {
		t.Fatal("second DTM enabled = false, want default true")
	}
	if secondCfg.DTM.Server != "http://localhost:36789" {
		t.Fatalf("second DTM server = %q, want http://localhost:36789", secondCfg.DTM.Server)
	}
}
