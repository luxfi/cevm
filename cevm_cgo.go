//go:build cgo

package cevm

/*
#cgo CFLAGS: -I${SRCDIR}/../../luxcpp/evm/lib/evm/gpu
#cgo LDFLAGS: -L${SRCDIR}/../../luxcpp/evm/build/lib
#cgo LDFLAGS: -L${SRCDIR}/../../luxcpp/evm/build/lib/evm
#cgo LDFLAGS: -L${SRCDIR}/../../luxcpp/evm/build/lib/evm/luxcpp-gpu
#cgo LDFLAGS: -Wl,-rpath,${SRCDIR}/../../luxcpp/evm/build/lib
#cgo LDFLAGS: -Wl,-rpath,${SRCDIR}/../../luxcpp/evm/build/lib/evm/luxcpp-gpu
#cgo LDFLAGS: -levm
#cgo darwin LDFLAGS: -levm-gpu -levm-metal-hosts -levm-kernel-metal -levm-gpu -lluxgpu -lstdc++
#cgo darwin LDFLAGS: -framework Metal -framework Foundation
#cgo linux  LDFLAGS: -Wl,--start-group -levm-gpu -lluxgpu -Wl,--end-group -lstdc++

#include <stdlib.h>
#include "go_bridge.h"
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// AutoDetect returns the best available backend for this machine.
func AutoDetect() Backend {
	return Backend(C.gpu_auto_detect_backend())
}

// init validates the ABI version of the loaded shared library against the Go
// module's expected ABIVersion. A mismatch means the binary and the cevm
// module were built against incompatible C++ headers and any execution would
// produce silently wrong results — fail fast at process start instead.
func init() {
	got := uint32(C.gpu_abi_version())
	if got != ABIVersion {
		panic(fmt.Sprintf(
			"cevm: ABI version mismatch — loaded libevm-gpu reports v%d but Go bindings expect v%d. "+
				"Rebuild libevm-gpu (see luxcpp/evm) or pin matching versions.",
			got, ABIVersion))
	}
}

// buildTxs converts Go transactions into C-layout transactions, pinning any
// Go-owned byte slices for the duration of the C call. The caller is
// responsible for invoking pinner.Unpin() once C has returned.
//
// Pinning rule: ctxs[i].data and ctxs[i].code are Go pointers inside Go
// memory that C will dereference. Per Go cgo rules these inner pointers
// MUST be pinned. ctxs[i].from / ctxs[i].to are stored by-value (array
// copy) so they don't need pinning.
func buildTxs(txs []Transaction, pinner *runtime.Pinner) []C.CGpuTx {
	ctxs := make([]C.CGpuTx, len(txs))
	for i := range txs {
		t := &txs[i]
		ctxs[i].from = *(*[20]C.uint8_t)(unsafe.Pointer(&t.From[0]))
		ctxs[i].to = *(*[20]C.uint8_t)(unsafe.Pointer(&t.To[0]))
		ctxs[i].gas_limit = C.uint64_t(t.GasLimit)
		ctxs[i].value = C.uint64_t(t.Value)
		ctxs[i].nonce = C.uint64_t(t.Nonce)
		ctxs[i].gas_price = C.uint64_t(t.GasPrice)
		if t.HasTo {
			ctxs[i].has_to = 1
		}
		if len(t.Data) > 0 {
			pinner.Pin(&t.Data[0])
			ctxs[i].data = (*C.uint8_t)(unsafe.Pointer(&t.Data[0]))
			ctxs[i].data_len = C.uint32_t(len(t.Data))
		}
		if len(t.Code) > 0 {
			pinner.Pin(&t.Code[0])
			ctxs[i].code = (*C.uint8_t)(unsafe.Pointer(&t.Code[0]))
			ctxs[i].code_len = C.uint32_t(len(t.Code))
		}
	}
	return ctxs
}

// copyU64 safely copies up to want elements from a C uint64 array into a Go
// slice. Bounds-checks `want` against a sane maximum to defend against a
// corrupted result struct returning an absurd count.
func copyU64(ptr *C.uint64_t, want uint32) []uint64 {
	if ptr == nil || want == 0 {
		return nil
	}
	const maxTxsPerBlock = 1 << 24 // 16M txs/block — far above any realistic block
	if want > maxTxsPerBlock {
		return nil
	}
	src := unsafe.Slice((*uint64)(unsafe.Pointer(ptr)), int(want))
	dst := make([]uint64, want)
	copy(dst, src)
	return dst
}

// ExecuteBlock runs a block of transactions through the C++ EVM.
//
// Thread safety: ExecuteBlock is safe to call from multiple goroutines
// concurrently. The C++ engine uses thread-local kernel hosts, so each
// goroutine that reaches the GPU path gets its own MTLBuffer/CUDA context
// cache. There are no shared mutable globals between calls.
func ExecuteBlock(backend Backend, txs []Transaction) (*BlockResult, error) {
	if len(txs) == 0 {
		return &BlockResult{}, nil
	}

	var pinner runtime.Pinner
	defer pinner.Unpin()
	ctxs := buildTxs(txs, &pinner)

	result := C.gpu_execute_block(
		&ctxs[0],
		C.uint32_t(len(ctxs)),
		C.uint8_t(backend),
	)
	// Always free the C-allocated result, even on the error path. The C
	// implementation of gpu_free_result is null-safe so this is a no-op
	// when result.gas_used is nil.
	defer C.gpu_free_result(&result)

	if result.ok == 0 {
		return nil, fmt.Errorf("cevm: execute_block failed")
	}

	return &BlockResult{
		GasUsed:      copyU64(result.gas_used, uint32(result.num_txs)),
		TotalGas:     uint64(result.total_gas),
		ExecTimeMs:   float64(result.exec_time_ms),
		Conflicts:    uint32(result.conflicts),
		ReExecutions: uint32(result.re_executions),
	}, nil
}

// ExecuteBlockV2 runs a block through the C++ EVM and returns the V2 result
// with per-tx status and post-execution state root.
//
// Thread safety: same as ExecuteBlock — safe under concurrent goroutines.
func ExecuteBlockV2(backend Backend, numThreads uint32, txs []Transaction) (*BlockResultV2, error) {
	if len(txs) == 0 {
		return &BlockResultV2{ABIVersion: ABIVersion}, nil
	}

	var pinner runtime.Pinner
	defer pinner.Unpin()
	ctxs := buildTxs(txs, &pinner)

	result := C.gpu_execute_block_v2(
		&ctxs[0],
		C.uint32_t(len(ctxs)),
		C.uint8_t(backend),
		C.uint32_t(numThreads),
	)
	defer C.gpu_free_result_v2(&result)

	if result.ok == 0 {
		return nil, fmt.Errorf("cevm: execute_block_v2 failed")
	}
	if uint32(result.abi_version) != ABIVersion {
		return nil, fmt.Errorf("cevm: ABI version mismatch in result (lib=%d expected=%d)",
			uint32(result.abi_version), ABIVersion)
	}

	br := &BlockResultV2{
		GasUsed:      copyU64(result.gas_used, uint32(result.num_txs)),
		TotalGas:     uint64(result.total_gas),
		ExecTimeMs:   float64(result.exec_time_ms),
		Conflicts:    uint32(result.conflicts),
		ReExecutions: uint32(result.re_executions),
		ABIVersion:   uint32(result.abi_version),
	}
	for i := 0; i < 32; i++ {
		br.StateRoot[i] = byte(result.state_root[i])
	}
	if result.status != nil && result.num_txs > 0 {
		const maxTxsPerBlock = 1 << 24
		want := uint32(result.num_txs)
		if want > maxTxsPerBlock {
			return nil, fmt.Errorf("cevm: result.num_txs=%d exceeds sanity bound", want)
		}
		statSlice := unsafe.Slice((*uint8)(unsafe.Pointer(result.status)), int(want))
		br.Status = make([]TxStatus, want)
		for i, s := range statSlice {
			br.Status[i] = TxStatus(s)
		}
	}
	return br, nil
}

// BackendName returns the human-readable name of a backend as reported by the
// C++ library (which is authoritative).
func BackendName(b Backend) string {
	cstr := C.gpu_backend_name(C.uint8_t(b))
	if cstr == nil {
		return "unknown"
	}
	return C.GoString(cstr)
}

// AvailableBackends returns the list of backends compiled and detected
// at runtime by the loaded library.
func AvailableBackends() []Backend {
	n := uint32(C.gpu_available_backends(nil, 0))
	if n == 0 {
		return nil
	}
	buf := make([]C.uint8_t, n)
	got := uint32(C.gpu_available_backends(&buf[0], C.uint32_t(n)))
	out := make([]Backend, got)
	for i := uint32(0); i < got; i++ {
		out[i] = Backend(buf[i])
	}
	return out
}

// LibraryABIVersion returns the ABI version reported by the loaded library.
// Useful for diagnostics when binaries and shared libs may drift.
func LibraryABIVersion() uint32 {
	return uint32(C.gpu_abi_version())
}

// healthBytecode returns the canonical health-check program: PUSH1 1, PUSH1 1,
// ADD, POP, PUSH1 0, PUSH1 0, RETURN. Every conformant EVM must execute this
// to completion with a deterministic gas cost.
//
// Expected behaviour:
//   - status == TxOK (0) for STOP — but RETURN-with-zero-bytes ⇒ TxReturn (1).
//   - gas_used > 0 (we charge GAS_VERYLOW * 6 + GAS_BASE * 2 = 22 minimum
//     plus the RETURN dispatch).
func healthBytecode() []byte {
	return []byte{
		0x60, 0x01, // PUSH1 1
		0x60, 0x01, // PUSH1 1
		0x01,       // ADD
		0x50,       // POP
		0x60, 0x00, // PUSH1 0
		0x60, 0x00, // PUSH1 0
		0xf3, // RETURN
	}
}

// HealthReport is the per-backend result of Health().
type HealthReport struct {
	Backend  Backend
	Name     string
	OK       bool
	Err      error
	GasUsed  uint64
	Status   TxStatus
	ExecTime float64
}

// Health runs the canonical health-check bytecode through every backend the
// loaded library exposes and returns a per-backend report. Use at process
// start to fail-fast on misconfigured GPUs (driver missing, library mismatch,
// device permissions). Returns nil only if the runtime cannot enumerate
// backends at all (catastrophic library failure).
//
// Returns one HealthReport per backend in the order reported by the library.
func Health() []HealthReport {
	backends := AvailableBackends()
	if len(backends) == 0 {
		return nil
	}
	out := make([]HealthReport, 0, len(backends))
	code := healthBytecode()
	tx := Transaction{
		HasTo:    true,
		Code:     code,
		GasLimit: 100_000,
		Nonce:    0,
		GasPrice: 1,
	}
	for _, b := range backends {
		rep := HealthReport{Backend: b, Name: BackendName(b)}
		r, err := ExecuteBlockV2(b, 0, []Transaction{tx})
		if err != nil {
			rep.Err = err
		} else if len(r.GasUsed) != 1 || len(r.Status) != 1 {
			rep.Err = fmt.Errorf("backend %s returned malformed result (gas=%d status=%d)",
				rep.Name, len(r.GasUsed), len(r.Status))
		} else if r.GasUsed[0] == 0 {
			rep.Err = fmt.Errorf("backend %s reported 0 gas — kernel did not execute", rep.Name)
		} else {
			rep.OK = true
			rep.GasUsed = r.GasUsed[0]
			rep.Status = r.Status[0]
			rep.ExecTime = r.ExecTimeMs
		}
		out = append(out, rep)
	}
	return out
}
