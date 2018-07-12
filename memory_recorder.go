package reporter

import (
	"fmt"
)

type MemoryRecorder struct{}

func (m *MemoryRecorder) Record(blk *Block) error {
	return fmt.Errorf("not implemented")
}

func (m *MemoryRecorder) Close() error {
	return nil
}
