package lb

import (
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
}

type Config struct {
	Port                string
	HealthCheckInterval string
	Servers             []string
}

func (lb *LoadBalancer) GetNextServer(servers []*Server) *Server {
	lb.Mutex.Lock()
	defer lb.Mutex.Unlock()

	for range servers {
		idx := lb.Current % len(servers)
		nextServer := servers[idx]
		lb.Current++

		nextServer.Mutex.Lock()
		isHealthy := nextServer.IsHealthy
		nextServer.Mutex.Unlock()

		if isHealthy {
			return nextServer
		}
	}

	return nil
}
