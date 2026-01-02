package runtime

import (
	"sync/atomic"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/engine"
)

type Snapshot struct {
	Cfg config.Config
	Eng *engine.Engine
}

type Runtime struct {
	cur atomic.Value
}

func New(initial Snapshot) *Runtime {
	r := &Runtime{}
	r.cur.Store(initial)
	return r
}

func (r *Runtime) Get() Snapshot {
	v := r.cur.Load()
	if v == nil {
		return Snapshot{}
	}
	return v.(Snapshot)
}

func (r *Runtime) Swap(next Snapshot) {
	r.cur.Store(next)
}
