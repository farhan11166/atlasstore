package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	fmt.Println("=== AtlasStore Load Test ===")
	
	// 1. Setup: Register and Login to get JWT
	email := fmt.Sprintf("loadtest_%d@test.com", time.Now().Unix())
	pass := "password"
	
	registerBody := map[string]string{"email": email, "password": pass}
	b, _ := json.Marshal(registerBody)
	
	resp, err := http.Post("http://localhost:8000/auth/register", "application/json", bytes.NewBuffer(b))
	if err != nil {
		fmt.Println("Gateway is down:", err)
		return
	}
	if resp.StatusCode >= 400 {
		// Try login if user exists
		resp, _ = http.Post("http://localhost:8000/auth/login", "application/json", bytes.NewBuffer(b))
	}
	
	var authRes struct { Token string }
	json.NewDecoder(resp.Body).Decode(&authRes)
	token := authRes.Token
	
	if token == "" {
		fmt.Println("Failed to get JWT token")
		return
	}
	fmt.Println("✓ Authenticated successfully")

	// 2. Configuration
	concurrency := 2000
	duration := 15 * time.Second
	
	var totalRequests int64
	var totalErrors int64
	
	fmt.Printf("Starting load test: %d concurrent users for %v...\n", concurrency, duration)
	
	startTime := time.Now()
	var wg sync.WaitGroup
	
	// 3. Fire requests
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			// Use a single HTTP client with keep-alive
			client := &http.Client{
				Transport: &http.Transport{
					MaxIdleConns:        100,
					MaxIdleConnsPerHost: 100,
				},
				Timeout: 10 * time.Second,
			}
			
			for time.Since(startTime) < duration {
				atomic.AddInt64(&totalRequests, 1)
				
				// Init multipart
				req, _ := http.NewRequest("POST", "http://localhost:8000/multipart", bytes.NewBuffer([]byte(`{"filename":"load.txt","content_type":"text/plain"}`)))
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				
				res, err := client.Do(req)
				if err != nil || res.StatusCode >= 400 {
					atomic.AddInt64(&totalErrors, 1)
					if res != nil { io.Copy(io.Discard, res.Body); res.Body.Close() }
					continue
				}
				
				var initRes struct { UploadID string `json:"upload_id"` }
				json.NewDecoder(res.Body).Decode(&initRes)
				res.Body.Close()
				uploadID := initRes.UploadID
				
				// Upload part
				req, _ = http.NewRequest("POST", fmt.Sprintf("http://localhost:8000/multipart/%s/0", uploadID), bytes.NewBuffer([]byte("load test chunk data")))
				req.Header.Set("Authorization", "Bearer "+token)
				res, err = client.Do(req)
				if err != nil || res.StatusCode >= 400 {
					atomic.AddInt64(&totalErrors, 1)
					if res != nil { io.Copy(io.Discard, res.Body); res.Body.Close() }
					continue
				}
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
				
				// Complete multipart
				req, _ = http.NewRequest("POST", fmt.Sprintf("http://localhost:8000/multipart/%s/complete", uploadID), nil)
				req.Header.Set("Authorization", "Bearer "+token)
				res, err = client.Do(req)
				if err != nil || res.StatusCode >= 400 {
					atomic.AddInt64(&totalErrors, 1)
				}
				if res != nil { io.Copy(io.Discard, res.Body); res.Body.Close() }
			}
		}(i)
	}
	
	wg.Wait()
	actualDuration := time.Since(startTime)
	
	// 4. Print results
	fmt.Println("\n=== Results ===")
	fmt.Printf("Total Requests (Uploads): %d\n", totalRequests)
	fmt.Printf("Total Errors:             %d\n", totalErrors)
	fmt.Printf("Success Rate:             %.2f%%\n", float64(totalRequests-totalErrors)/float64(totalRequests)*100)
	fmt.Printf("Throughput (Req/Sec):     %.2f\n", float64(totalRequests)/actualDuration.Seconds())
}
