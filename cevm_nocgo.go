//go:build !cgo

package cevm

import "fmt"

// AutoDetect returns CPUSequential when built without CGo.
// The actual auto-detection requires the C++ library.
func AutoDetect() Backend {
	return CPUSequential
}

// ExecuteBlock returns an error when built without CGo.
// Build with CGO_ENABLED=1 and link against libevm to enable execution.
func ExecuteBlock(backend Backend, txs []Transaction) (*BlockResult, error) {
	if len(txs) == 0 {
		return &BlockResult{}, nil
	}
	return nil, fmt.Errorf("cevm: built without CGo, cannot execute transactions (rebuild with CGO_ENABLED=1)")
}
