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

func TestTxStatusString(t *testing.T) {
	tests := []struct {
		s    TxStatus
		want string
	}{
		{TxOK, "ok"},
		{TxReturn, "return"},
		{TxRevert, "revert"},
		{TxOOG, "oog"},
		{TxError, "error"},
		{TxCallNotSupported, "call-not-supported"},
		{TxStatus(99), "status(99)"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("TxStatus(%d).String() = %q, want %q", int(tt.s), got, tt.want)
		}
	}
}

func TestExecuteBlockV2Empty(t *testing.T) {
	result, err := ExecuteBlockV2(CPUSequential, 0, nil)
	if err != nil {
		t.Fatalf("ExecuteBlockV2(nil) returned error: %v", err)
	}
	if result.ABIVersion != ABIVersion {
		t.Errorf("ABIVersion = %d, want %d", result.ABIVersion, ABIVersion)
	}
	if result.TotalGas != 0 {
		t.Errorf("expected 0 total gas for empty block, got %d", result.TotalGas)
	}
}

func TestAvailableBackends(t *testing.T) {
	bs := AvailableBackends()
	if len(bs) == 0 {
		t.Fatal("AvailableBackends() returned empty list")
	}
	// CPUSequential must always be available.
	hasSeq := false
	for _, b := range bs {
		if b == CPUSequential {
			hasSeq = true
		}
	}
	if !hasSeq {
		t.Errorf("AvailableBackends() missing CPUSequential: %v", bs)
	}
}

func TestBackendName(t *testing.T) {
	// BackendName goes through the loaded library (cgo path) or local
	// Go strings (nocgo path). Either way, it must produce a non-empty
	// string for known backends.
	for _, b := range []Backend{CPUSequential, CPUParallel, GPUMetal, GPUCUDA} {
		if BackendName(b) == "" {
			t.Errorf("BackendName(%d) is empty", int(b))
		}
	}
}

func TestLibraryABIVersion(t *testing.T) {
	got := LibraryABIVersion()
	if got != ABIVersion {
		t.Errorf("LibraryABIVersion() = %d, want %d (rebuild libevm-gpu)", got, ABIVersion)
	}
}

func smokeTx(i uint64) Transaction {
	var from [20]byte
	from[19] = byte(i + 1) // distinct sender per tx
	return Transaction{
		From:     from,
		HasTo:    true,
		GasLimit: 21000,
		Value:    1,
		Nonce:    i,
		GasPrice: 1,
	}
}

func TestExecuteBlockSmoke_AllBackends(t *testing.T) {
	// Send a tiny block through every backend the loaded library exposes.
	// Each backend must produce a result with TotalGas > 0 and num_txs entries.
	const N = 4
	txs := make([]Transaction, N)
	for i := range txs {
		txs[i] = smokeTx(uint64(i))
	}

	for _, b := range AvailableBackends() {
		t.Run(BackendName(b), func(t *testing.T) {
			r, err := ExecuteBlock(b, txs)
			if err != nil {
				t.Fatalf("ExecuteBlock(%s): %v", BackendName(b), err)
			}
			if r.TotalGas == 0 {
				t.Errorf("expected non-zero total gas, got 0")
			}
			if len(r.GasUsed) != N {
				t.Errorf("len(GasUsed) = %d, want %d", len(r.GasUsed), N)
			}
		})
	}
}

func TestExecuteBlockV2Smoke_AllBackends(t *testing.T) {
	const N = 4
	txs := make([]Transaction, N)
	for i := range txs {
		txs[i] = smokeTx(uint64(i))
	}
	for _, b := range AvailableBackends() {
		t.Run(BackendName(b), func(t *testing.T) {
			r, err := ExecuteBlockV2(b, 0, txs)
			if err != nil {
				t.Fatalf("ExecuteBlockV2(%s): %v", BackendName(b), err)
			}
			if r.ABIVersion != ABIVersion {
				t.Errorf("ABIVersion = %d, want %d", r.ABIVersion, ABIVersion)
			}
			if len(r.GasUsed) != N {
				t.Errorf("len(GasUsed) = %d, want %d", len(r.GasUsed), N)
			}
			if len(r.Status) != N {
				t.Errorf("len(Status) = %d, want %d", len(r.Status), N)
			}
		})
	}
}
