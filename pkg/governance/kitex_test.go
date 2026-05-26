package governance

import (
	"ClaranAIM/pkg/config"
	"testing"
	"time"
)

func TestOrdinaryRPCUsesDefaultTimeoutWhenConfigIsEmpty(t *testing.T) {
	timeout, enabled := clientTimeout(config.RPCGovernanceConfig{}, false)
	if !enabled {
		t.Fatal("ordinary RPC timeout disabled, want default timeout")
	}
	if timeout != 5*time.Second {
		t.Fatalf("ordinary RPC timeout = %v, want 5s", timeout)
	}
}

func TestLongRunningRPCCanDisableTimeout(t *testing.T) {
	timeout, enabled := clientTimeout(config.RPCGovernanceConfig{TimeoutMS: 0}, true)
	if enabled {
		t.Fatalf("long-running RPC timeout enabled with %v, want disabled", timeout)
	}
}

func TestLongRunningRPCUsesConfiguredPositiveTimeout(t *testing.T) {
	timeout, enabled := clientTimeout(config.RPCGovernanceConfig{TimeoutMS: 120000}, true)
	if !enabled {
		t.Fatal("long-running RPC timeout disabled, want configured timeout")
	}
	if timeout != 120*time.Second {
		t.Fatalf("long-running RPC timeout = %v, want 120s", timeout)
	}
}
