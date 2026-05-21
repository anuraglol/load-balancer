package lb

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
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
