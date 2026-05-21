package metrics

import (
	"net"
	"net/http"
	"sync/atomic"
)

type ConnTracker struct {
	activeConns int64
}

func NewConnTracker() *ConnTracker {
	return &ConnTracker{}
}

func (c *ConnTracker) HandleStateChange(conn net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		atomic.AddInt64(&c.activeConns, 1)
	case http.StateClosed, http.StateHijacked:
		atomic.AddInt64(&c.activeConns, -1)
	}
}

func (c *ConnTracker) ActiveConns() int64 {
	return atomic.LoadInt64(&c.activeConns)
}
