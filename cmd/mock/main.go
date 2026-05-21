package main

import (
	"fmt"
	"lb/metrics"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

type connTracker struct {
	activeConns int64
}

func (c *connTracker) handleStateChange(conn net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		atomic.AddInt64(&c.activeConns, 1)
	case http.StateClosed, http.StateHijacked:
		atomic.AddInt64(&c.activeConns, -1)
	}
}

func main() {
	ports := []string{":5001", ":5002", ":5003", ":5004", ":5005"}

	for _, port := range ports {
		tracker := metrics.NewConnTracker()
		p := port

		go func() {
			mux := http.NewServeMux()

			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				current := atomic.LoadInt64(&tracker.activeConns)
				fmt.Fprintf(w, "Active connections: %d\n", current)
				fmt.Fprintf(w, "Response from backend server on port %s\n", p)
			})

			mux.HandleFunc("/stress", func(w http.ResponseWriter, r *http.Request) {
				jitter := rand.Intn(300)
				time.Sleep(time.Duration(jitter) * time.Millisecond)

				if rand.Float32() < 0.1 {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(w, "Server on port %s failed under load!\n", p)
					return
				}

				fmt.Fprintf(w, "Processed heavy request on port %s in %dms\n", p, jitter)
			})

			server := &http.Server{
				Handler:   mux,
				ConnState: tracker.handleStateChange,
				Addr:      port,
			}

			log.Printf("Starting mock backend server on %s\n", p)
			if err := server.ListenAndServe(); err != nil {
				log.Printf("Server on port %s crashed: %v\n", p, err)
			}
		}()
	}

	select {}
}
