package main

import (
	"fmt"
	"lb"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

func healthCheck(s *lb.Server, interval time.Duration) {
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

func main() {
	balancer := &lb.LoadBalancer{Current: 0}
	ports := []string{"http://localhost:5001", "http://localhost:5002"}
	var servers []*lb.Server

	for _, u := range ports {
		parsedURL, _ := url.Parse(u)
		server := &lb.Server{
			IsHealthy:    true,
			URL:          parsedURL,
			ReverseProxy: httputil.NewSingleHostReverseProxy(parsedURL),
		}
		servers = append(servers, server)
		go healthCheck(server, 2*time.Second)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s := balancer.GetNextServer(servers)
		if s == nil {
			http.Error(w, "No healthy server available", http.StatusServiceUnavailable)
			return
		}
		s.ReverseProxy.ServeHTTP(w, r)
	})

	log.Println("Starting load balancer on :8000")
	http.ListenAndServe(":8000", nil)
}
