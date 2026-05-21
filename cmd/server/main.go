package main

import (
	"context"
	"fmt"
	"lb"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	balancer := &lb.LoadBalancer{Current: 0}

	svs := []struct {
		URL    string
		Weight int
	}{
		{URL: "http://localhost:5001", Weight: rand.Intn(10) + 1},
		{URL: "http://localhost:5002", Weight: rand.Intn(10) + 1},
		{URL: "http://localhost:5003", Weight: rand.Intn(10) + 1},
		{URL: "http://localhost:5004", Weight: rand.Intn(10) + 1},
	}
	var servers []*lb.Server

	var wg sync.WaitGroup
	var activeHttpServers []*http.Server

	for _, sv := range svs {
		parsedURL, _ := url.Parse(sv.URL)
		server := &lb.Server{
			IsHealthy:    true,
			URL:          parsedURL,
			ReverseProxy: httputil.NewSingleHostReverseProxy(parsedURL),
			Connections:  0,
			Weight:       int32(sv.Weight),
		}
		servers = append(servers, server)
		go lb.HealthCheck(server, 2*time.Second)
	}

	for _, server := range servers {
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

		activeHttpServers = append(activeHttpServers, httpServer)

		wg.Add(1)
		go func(s *http.Server, p string) {
			defer wg.Done()
			log.Printf("Starting mock backend server on %s\n", p)
			if err := s.ListenAndServe(); err != http.ErrServerClosed {
				log.Printf("Server on port %s crashed: %v\n", p, err)
			}
		}(httpServer, port)
	}

	mainMux := http.NewServeMux()
	mainMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s := balancer.GetNextServer(servers)
		if s == nil {
			http.Error(w, "No healthy server available", http.StatusServiceUnavailable)
			return
		}
		s.ReverseProxy.ServeHTTP(w, r)
	})

	lbServer := &http.Server{
		Addr:    ":8000",
		Handler: mainMux,
	}

	go func() {
		log.Println("Starting load balancer on :8000")
		if err := lbServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Load balancer failed: %v\n", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("\n[Ctrl+C] Initiating graceful shutdown...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := lbServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Load balancer shutdown error: %v\n", err)
	}

	for _, s := range activeHttpServers {
		if err := s.Shutdown(shutdownCtx); err != nil {
			log.Printf("Backend server shutdown error: %v\n", err)
		}
	}

	wg.Wait()
	log.Println("All systems shut down cleanly. bye bye :3!")
}
