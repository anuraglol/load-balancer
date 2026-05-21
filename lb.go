package lb

import (
	"lb/metrics"
	"net/http/httputil"
	"net/url"
	"sync"
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
	Tracker      *metrics.ConnTracker
}

type Config struct {
	Port                string
	HealthCheckInterval string
	Servers             []string
}

func (lb *LoadBalancer) GetNextServer(servers []*Server) *Server {
	lb.Mutex.Lock()
	defer lb.Mutex.Unlock()

	// for range servers {
	// 	idx := lb.Current % len(servers)
	// 	nextServer := servers[idx]
	// 	lb.Current++

	// 	nextServer.Mutex.Lock()
	// 	isHealthy := nextServer.IsHealthy
	// 	nextServer.Mutex.Unlock()

	// 	if isHealthy {
	// 		return nextServer
	// 	}
	// }

	minConnectionsIdx := 0
	for idx, sv := range servers {
		if sv.Tracker.ActiveConns() < servers[minConnectionsIdx].Tracker.ActiveConns() {
			minConnectionsIdx = idx
		}
	}
	minStressedServer := servers[minConnectionsIdx]
	minStressedServer.Mutex.Lock()
	isHealthy := minStressedServer.IsHealthy
	minStressedServer.Mutex.Unlock()
	lb.Current = minConnectionsIdx

	if isHealthy {
		return minStressedServer
	}

	return nil
}
