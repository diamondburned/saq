package atomicg

import "sync/atomic"

type Bool struct {
	u uint32
}

func (b *Bool) Set() {
	atomic.StoreUint32(&b.u, 1)
}

func (b *Bool) Unset() {
	atomic.StoreUint32(&b.u, 0)
}

func (b *Bool) IsSet() bool {
	return atomic.LoadUint32(&b.u) == 1
}
