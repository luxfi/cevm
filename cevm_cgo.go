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
#cgo darwin LDFLAGS: -levm-gpu -levm-metal-hosts -levm-gpu -lluxgpu -lstdc++
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

// ExecuteBlock runs a block of transactions through the C++ EVM.
func ExecuteBlock(backend Backend, txs []Transaction) (*BlockResult, error) {
	if len(txs) == 0 {
		return &BlockResult{}, nil
	}

	ctxs := make([]C.CGpuTx, len(txs))
	// Pin every Go-owned slice that ends up being addressed from inside ctxs.
	// cgo forbids "Go pointer to unpinned Go pointer" — the ctxs array itself
	// is passed to C, and each entry's `data` field is a Go pointer.
	var pinner runtime.Pinner
	defer pinner.Unpin()

	for i, tx := range txs {
		ctxs[i].from = *(*[20]C.uint8_t)(unsafe.Pointer(&tx.From[0]))
		ctxs[i].to = *(*[20]C.uint8_t)(unsafe.Pointer(&tx.To[0]))
		ctxs[i].gas_limit = C.uint64_t(tx.GasLimit)
		ctxs[i].value = C.uint64_t(tx.Value)
		ctxs[i].nonce = C.uint64_t(tx.Nonce)
		ctxs[i].gas_price = C.uint64_t(tx.GasPrice)

		if tx.HasTo {
			ctxs[i].has_to = 1
		}

		if len(tx.Data) > 0 {
			pinner.Pin(&txs[i].Data[0])
			ctxs[i].data = (*C.uint8_t)(unsafe.Pointer(&txs[i].Data[0]))
			ctxs[i].data_len = C.uint32_t(len(tx.Data))
		}
	}

	result := C.gpu_execute_block(
		&ctxs[0],
		C.uint32_t(len(ctxs)),
		C.uint8_t(backend),
	)

	if result.ok == 0 {
		return nil, fmt.Errorf("cevm: execute_block failed")
	}

	br := &BlockResult{
		TotalGas:     uint64(result.total_gas),
		ExecTimeMs:   float64(result.exec_time_ms),
		Conflicts:    uint32(result.conflicts),
		ReExecutions: uint32(result.re_executions),
	}

	if result.gas_used != nil && result.num_txs > 0 {
		gasSlice := unsafe.Slice((*uint64)(unsafe.Pointer(result.gas_used)), int(result.num_txs))
		br.GasUsed = make([]uint64, len(gasSlice))
		copy(br.GasUsed, gasSlice)
	}

	C.gpu_free_result(&result)

	return br, nil
}

// ExecuteBlockV2 runs a block through the C++ EVM and returns the V2 result
// with per-tx status and post-execution state root.
func ExecuteBlockV2(backend Backend, numThreads uint32, txs []Transaction) (*BlockResultV2, error) {
	if got := uint32(C.gpu_abi_version()); got != ABIVersion {
		return nil, fmt.Errorf("cevm: ABI version mismatch (lib=%d expected=%d)", got, ABIVersion)
	}
	if len(txs) == 0 {
		return &BlockResultV2{ABIVersion: ABIVersion}, nil
	}

	ctxs := make([]C.CGpuTx, len(txs))
	var pinner runtime.Pinner
	defer pinner.Unpin()

	for i, tx := range txs {
		ctxs[i].from = *(*[20]C.uint8_t)(unsafe.Pointer(&tx.From[0]))
		ctxs[i].to = *(*[20]C.uint8_t)(unsafe.Pointer(&tx.To[0]))
		ctxs[i].gas_limit = C.uint64_t(tx.GasLimit)
		ctxs[i].value = C.uint64_t(tx.Value)
		ctxs[i].nonce = C.uint64_t(tx.Nonce)
		ctxs[i].gas_price = C.uint64_t(tx.GasPrice)
		if tx.HasTo {
			ctxs[i].has_to = 1
		}
		if len(tx.Data) > 0 {
			pinner.Pin(&txs[i].Data[0])
			ctxs[i].data = (*C.uint8_t)(unsafe.Pointer(&txs[i].Data[0]))
			ctxs[i].data_len = C.uint32_t(len(tx.Data))
		}
	}

	result := C.gpu_execute_block_v2(
		&ctxs[0],
		C.uint32_t(len(ctxs)),
		C.uint8_t(backend),
		C.uint32_t(numThreads),
	)

	if result.ok == 0 {
		C.gpu_free_result_v2(&result)
		return nil, fmt.Errorf("cevm: execute_block_v2 failed")
	}

	br := &BlockResultV2{
		TotalGas:     uint64(result.total_gas),
		ExecTimeMs:   float64(result.exec_time_ms),
		Conflicts:    uint32(result.conflicts),
		ReExecutions: uint32(result.re_executions),
		ABIVersion:   uint32(result.abi_version),
	}
	for i := 0; i < 32; i++ {
		br.StateRoot[i] = byte(result.state_root[i])
	}
	if result.gas_used != nil && result.num_txs > 0 {
		gasSlice := unsafe.Slice((*uint64)(unsafe.Pointer(result.gas_used)), int(result.num_txs))
		br.GasUsed = make([]uint64, len(gasSlice))
		copy(br.GasUsed, gasSlice)
	}
	if result.status != nil && result.num_txs > 0 {
		statSlice := unsafe.Slice((*uint8)(unsafe.Pointer(result.status)), int(result.num_txs))
		br.Status = make([]TxStatus, len(statSlice))
		for i, s := range statSlice {
			br.Status[i] = TxStatus(s)
		}
	}

	C.gpu_free_result_v2(&result)
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
