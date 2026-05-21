package main

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type LoadBalancer struct {
	Current int
	Mutex   sync.Mutex
}

type Server struct {
	URL          *url.URL
	IsHealthy    bool
	ReverseProxy *httputil.ReverseProxy
	Mutex        sync.Mutex
	Connections  int32
	Weight       int32 // more weight = can handle more load
}

type Config struct {
	Port                string
	HealthCheckInterval string
	Servers             []string
}

func calcScore(sv *Server) int32 {
	return sv.Connections / sv.Weight
}

func HealthCheck(s *Server, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	client := &http.Client{Timeout: 2 * time.Second}

	for range ticker.C {
		res, err := client.Head(s.URL.String())
		s.Mutex.Lock()
		if err != nil || res.StatusCode != http.StatusOK {
			fmt.Printf("oops, server down :(: %v\n", s.URL.String())
			s.IsHealthy = false
		} else {
			s.IsHealthy = true
		}
		s.Mutex.Unlock()
	}
}

func (lb *LoadBalancer) GetNextServer(servers []*Server) *Server {
	lb.Mutex.Lock()
	defer lb.Mutex.Unlock()

	// score = active-connections/weight, min score := to pick next
	minScoreIdx := 0
	for idx, sv := range servers {
		if calcScore(sv) <= calcScore(servers[minScoreIdx]) {
			minScoreIdx = idx
		}
	}

	minStressedServer := servers[minScoreIdx]
	minStressedServer.Mutex.Lock()
	isHealthy := minStressedServer.IsHealthy
	minStressedServer.Mutex.Unlock()
	lb.Current = minScoreIdx

	if isHealthy {
		return minStressedServer
	}

	return nil
}

func PrepServer(server *Server) (*http.Server, string) {
	port := server.URL.Port()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conns := atomic.LoadInt32(&server.Connections)
		fmt.Fprintf(w, "Active connections: %d\n", conns)
		fmt.Fprintf(w, "Response from backend server on port %s\n", port)
	})

	mux.HandleFunc("/stress", func(w http.ResponseWriter, r *http.Request) {
		jitter := rand.Intn(300)
		time.Sleep(time.Duration(jitter) * time.Millisecond)

		if rand.Float32() < 0.1 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Server on port %s failed under load!\n", port)
			return
		}
		fmt.Fprintf(w, "Processed heavy request on port %s in %dms\n", port, jitter)
	})

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
		ConnState: func(conn net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				atomic.AddInt32(&server.Connections, 1)
			case http.StateClosed:
				atomic.AddInt32(&server.Connections, -1)
			}
		},
	}

	return httpServer, port
}
