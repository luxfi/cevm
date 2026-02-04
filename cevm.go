// Package cevm provides Go bindings to the C++ EVM (evmone fork with GPU acceleration).
// Import this package to use the C++ EVM as a drop-in replacement for go-ethereum's EVM.
//
// The C++ EVM supports:
//   - Block-STM parallel execution
//   - GPU Keccak-256 state hashing (Metal/CUDA)
//   - GPU batch ecrecover (Metal/CUDA)
//   - GPU EVM opcode interpreter (Metal/CUDA)
//   - ZAP VM plugin protocol (native)
//
// Build with CGo: CGO_ENABLED=1 go build -tags cgo
// Build without CGo: CGO_ENABLED=0 go build (types only, no execution)
// Binary: the `cevm` binary in luxcpp/evm/build/bin/ is the Lux VM plugin.
package cevm

import "fmt"

// Backend selects the C++ EVM execution mode.
type Backend int

const (
	// CPUSequential runs transactions one at a time on a single core.
	CPUSequential Backend = 0
	// CPUParallel uses Block-STM to run transactions across all cores.
	CPUParallel Backend = 1
	// GPUMetal offloads Keccak, ecrecover, and the EVM interpreter to Metal.
	GPUMetal Backend = 2
	// GPUCUDA offloads Keccak, ecrecover, and the EVM interpreter to CUDA.
	GPUCUDA Backend = 3
)

// String returns the human-readable name of the backend.
func (b Backend) String() string {
	switch b {
	case CPUSequential:
		return "cpu-sequential"
	case CPUParallel:
		return "cpu-parallel"
	case GPUMetal:
		return "gpu-metal"
	case GPUCUDA:
		return "gpu-cuda"
	default:
		return fmt.Sprintf("unknown(%d)", int(b))
	}
}

// Transaction is a single EVM transaction to execute.
type Transaction struct {
	From     [20]byte
	To       [20]byte
	HasTo    bool
	Data     []byte
	GasLimit uint64
	Value    uint64
	Nonce    uint64
	GasPrice uint64
}

// BlockResult holds the outcome of executing a block of transactions.
type BlockResult struct {
	// GasUsed per transaction, indexed by position.
	GasUsed []uint64
	// TotalGas consumed by the entire block.
	TotalGas uint64
	// ExecTimeMs is wall-clock execution time in milliseconds.
	ExecTimeMs float64
	// Conflicts detected during Block-STM parallel execution.
	Conflicts uint32
	// ReExecutions caused by conflicts.
	ReExecutions uint32
}
