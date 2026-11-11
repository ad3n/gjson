package gjson

import "sync"

const (
	joinPoolMaxInput   = 16 * 1024
	joinPoolMaxEntries = 256
)

type joinBuffer struct {
	values map[string]Result
	keys   []Result
}

var joinBufferPool = sync.Pool{
	New: func() any { return &joinBuffer{values: make(map[string]Result)} },
}

func acquireJoinBuffer(size int) *joinBuffer {
	if size <= joinPoolMaxInput {
		return joinBufferPool.Get().(*joinBuffer)
	}

	return &joinBuffer{values: make(map[string]Result)}
}

func releaseJoinBuffer(buffer *joinBuffer) {
	if len(buffer.values) > joinPoolMaxEntries || cap(buffer.keys) > joinPoolMaxEntries {
		buffer.values = nil
		buffer.keys = nil
		return
	}

	clear(buffer.values)
	clear(buffer.keys)
	buffer.keys = buffer.keys[:0]
	joinBufferPool.Put(buffer)
}
