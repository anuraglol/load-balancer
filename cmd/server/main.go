package main

import (
	"fmt"
	"lb"
	"log"
	"math/rand"
	"net"
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
	ports := []string{"http://localhost:5001", "http://localhost:5002", "http://localhost:5003", "http://localhost:5004"}
	var servers []*lb.Server

	for _, u := range ports {
		parsedURL, _ := url.Parse(u)
		server := &lb.Server{
			IsHealthy:    true,
			URL:          parsedURL,
			ReverseProxy: httputil.NewSingleHostReverseProxy(parsedURL),
			Connections:  0,
		}
		servers = append(servers, server)
		go healthCheck(server, 2*time.Second)
	}

	for _, server := range servers {
		port := server.URL.Port()
		go func(s *lb.Server) {
			mux := http.NewServeMux()

			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "Active connections: %d\n", server.Connections)
				fmt.Fprintf(w, "Response from backend server on port %s\n", port)
			})

			mux.HandleFunc("/stress", func(w http.ResponseWriter, r *http.Request) {
				fmt.Printf("current connections to server with port %v is: %v\n", port, s.Connections)
				jitter := rand.Intn(300)
				time.Sleep(time.Duration(jitter) * time.Millisecond)

				if rand.Float32() < 0.1 {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(w, "Server on port %s failed under load!\n", port)
					return
				}

				fmt.Fprintf(w, "Processed heavy request on port %s in %dms, when number of active connections is: %d\n", port, jitter, s.Connections)
			})

			httpServer := &http.Server{
				Handler: mux,
				ConnState: func(conn net.Conn, state http.ConnState) {
					switch state {
					case http.StateNew:
						server.Connections++
					case http.StateClosed:
						server.Connections--
					}
				},
				Addr: ":" + port,
			}

			log.Printf("Starting mock backend server on %s\n", port)
			if err := httpServer.ListenAndServe(); err != nil {
				log.Printf("Server on port %s crashed: %v\n", port, err)
			}
		}(server)
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
