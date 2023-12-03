package atomicg

import "sync/atomic"

type Int struct {
	i int64
}

func (b *Int) Set(v int64) {
	atomic.StoreInt64(&b.i, v)
}

func (b *Int) Get() int64 {
	return atomic.LoadInt64(&b.i)
}

func (b *Int) Add(v int64) int64 {
	return atomic.AddInt64(&b.i, v)
}

func (b *Int) Swap(v int64) int64 {
	return atomic.SwapInt64(&b.i, v)
}

func (b *Int) CompareAndSwap(old, new int64) bool {
	return atomic.CompareAndSwapInt64(&b.i, old, new)
}
