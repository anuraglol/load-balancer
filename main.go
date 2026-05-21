package main

import (
	"context"
	"log"
	"math/rand"
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
	balancer := &LoadBalancer{Current: 0}

	svs := []struct {
		URL    string
		Weight int
	}{
		{URL: "http://localhost:5001", Weight: rand.Intn(10) + 1},
		{URL: "http://localhost:5002", Weight: rand.Intn(10) + 1},
		{URL: "http://localhost:5003", Weight: rand.Intn(10) + 1},
		{URL: "http://localhost:5004", Weight: rand.Intn(10) + 1},
	}
	var servers []*Server

	var wg sync.WaitGroup
	var activeHttpServers []*http.Server

	for _, sv := range svs {
		parsedURL, _ := url.Parse(sv.URL)
		server := &Server{
			IsHealthy:    true,
			URL:          parsedURL,
			ReverseProxy: httputil.NewSingleHostReverseProxy(parsedURL),
			Connections:  0,
			Weight:       int32(sv.Weight),
		}
		servers = append(servers, server)
		go HealthCheck(server, 2*time.Second)
	}

	for _, server := range servers {
		httpServer, port := PrepServer(server)
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

	mainMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		type ServerStat struct {
			URL   string
			Count uint64
		}

		var stats []ServerStat
		var maxCount uint64

		for _, s := range servers {
			count := atomic.LoadUint64(&s.TotalRequests)
			if count > maxCount {
				maxCount = count
			}
			stats = append(stats, ServerStat{
				URL:   s.URL.String(),
				Count: count,
			})
		}

		_, _ = w.Write([]byte("histogram:\n"))

		for _, stat := range stats {
			var barLength int
			if maxCount > 0 {
				barLength = int((float64(stat.Count) / float64(maxCount)) * 40)
			}

			bar := ""
			for b := 0; b < barLength; b++ {
				bar += "■"
			}

			urlField := stat.URL
			for len(urlField) < 25 {
				urlField += " "
			}

			countField := "["

			line := urlField + " " + countField + "Hits" + "] | " + bar + "\n"
			_, _ = w.Write([]byte(line))
		}
	})

	mainMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s := balancer.GetNextServer(servers)
		if s == nil {
			http.Error(w, "No healthy server available", http.StatusServiceUnavailable)
			return
		}

		atomic.AddUint64(&s.TotalRequests, 1)
		s.ReverseProxy.ServeHTTP(w, r)
	})
	rateLimter := NewRateLimitMiddleware(10, 20)
	handler := rateLimter.Middleware(mainMux)

	lbServer := &http.Server{
		Addr:    ":8000",
		Handler: handler,
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
	log.Println("\n[Ctrl+C] initiating graceful shutdown...")

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
