//go:build cgo

package cevm

// #cgo CFLAGS: -I${SRCDIR}/../luxcpp/evm/lib/evm/gpu
// #cgo LDFLAGS: -L${SRCDIR}/../luxcpp/evm/build/lib -levm -levm-gpu
// #include "go_bridge.h"
import "C"

import (
	"fmt"
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
	// Pin data slices so GC does not move them during the C call.
	pins := make([][]byte, len(txs))

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
			pins[i] = tx.Data
			ctxs[i].data = (*C.uint8_t)(unsafe.Pointer(&pins[i][0]))
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
