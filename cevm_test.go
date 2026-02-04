package cevm

import (
	"testing"
)

func TestBackendString(t *testing.T) {
	tests := []struct {
		b    Backend
		want string
	}{
		{CPUSequential, "cpu-sequential"},
		{CPUParallel, "cpu-parallel"},
		{GPUMetal, "gpu-metal"},
		{GPUCUDA, "gpu-cuda"},
		{Backend(99), "unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.b.String(); got != tt.want {
			t.Errorf("Backend(%d).String() = %q, want %q", int(tt.b), got, tt.want)
		}
	}
}

func TestExecuteBlockEmpty(t *testing.T) {
	result, err := ExecuteBlock(CPUSequential, nil)
	if err != nil {
		t.Fatalf("ExecuteBlock(nil) returned error: %v", err)
	}
	if result.TotalGas != 0 {
		t.Errorf("expected 0 total gas for empty block, got %d", result.TotalGas)
	}
}

func TestVMID(t *testing.T) {
	if got := VMID(); got != "cevm" {
		t.Errorf("VMID() = %q, want %q", got, "cevm")
	}
}

func TestPluginPath(t *testing.T) {
	p := PluginPath()
	if p == "" {
		t.Skip("could not determine home directory")
	}
	// Just verify the path ends correctly.
	const suffix = "work/luxcpp/evm/build/bin/cevm"
	if len(p) < len(suffix) {
		t.Fatalf("PluginPath() too short: %q", p)
	}
	got := p[len(p)-len(suffix):]
	if got != suffix {
		t.Errorf("PluginPath() suffix = %q, want %q", got, suffix)
	}
}

func TestPluginExists(t *testing.T) {
	// This test just verifies the function runs without panicking.
	// Whether it returns true or false depends on the build environment.
	_ = PluginExists()
}
