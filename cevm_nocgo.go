//go:build !cgo

package cevm

import "fmt"

// AutoDetect returns CPUSequential when built without CGo.
func AutoDetect() Backend { return CPUSequential }

// AvailableBackends returns CPUSequential only when built without CGo.
func AvailableBackends() []Backend { return []Backend{CPUSequential} }

// BackendName uses the local Go-side string when CGo is off.
func BackendName(b Backend) string { return b.String() }

// LibraryABIVersion returns the Go-side constant when there's no library.
func LibraryABIVersion() uint32 { return ABIVersion }

// ExecuteBlock returns an error when built without CGo.
func ExecuteBlock(backend Backend, txs []Transaction) (*BlockResult, error) {
	if len(txs) == 0 {
		return &BlockResult{}, nil
	}
	return nil, fmt.Errorf("cevm: built without CGo, cannot execute transactions (rebuild with CGO_ENABLED=1)")
}

// ExecuteBlockV2 returns an error when built without CGo.
func ExecuteBlockV2(backend Backend, numThreads uint32, txs []Transaction) (*BlockResultV2, error) {
	if len(txs) == 0 {
		return &BlockResultV2{ABIVersion: ABIVersion}, nil
	}
	return nil, fmt.Errorf("cevm: built without CGo, cannot execute transactions (rebuild with CGO_ENABLED=1)")
}
