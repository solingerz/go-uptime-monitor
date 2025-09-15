package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	Port           string
	WorkerCount    int
	CheckInterval  time.Duration
	AggInterval    time.Duration
	RequestTimeout time.Duration
}

var config Config

var targetsMutex sync.RWMutex

type Target struct {
	Name string `json:"Name"`
	URL  string `json:"URL"`
}

var ctx = context.Background()
var rdb *redis.Client

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   false,
	},
}

var targets []Target

func loadConfig() {
	var newTargets []Target
	var err error

	targetsJSON := os.Getenv("UPTIME_TARGETS_JSON")
	if targetsJSON != "" {
		log.Println("Loading monitoring targets from UPTIME_TARGETS_JSON environment variable...")
		err = json.Unmarshal([]byte(targetsJSON), &newTargets)
		if err != nil {
			log.Fatalf("Failed to parse UPTIME_TARGETS_JSON: %v", err)
		}
	} else {
		log.Println("Loading monitoring targets from config.json file...")
		file, errFile := os.ReadFile("config.json")
		if errFile != nil {
			log.Fatalf("Failed to read config file 'config.json': %v", errFile)
		}
		err = json.Unmarshal(file, &newTargets)
		if err != nil {
			log.Fatalf("Failed to parse config file 'config.json': %v", err)
		}
	}

	for i, target := range newTargets {
		if target.Name == "" {
			log.Fatalf("Configuration error: target %d is missing a name", i+1)
		}
		if target.URL == "" {
			log.Fatalf("Configuration error: target '%s' is missing a URL", target.Name)
		}
	}

	targetsMutex.Lock()
	targets = newTargets
	targetsMutex.Unlock()

	log.Printf("Successfully loaded %d monitoring targets", len(newTargets))
}

func addRecordToRedis(targetName, status string, statusCode int, responseTimeMs int64) {
	streamKey := fmt.Sprintf("uptime:raw:%s", targetName)
	now := time.Now()
	
	values := map[string]interface{}{
		"status":           status,
		"status_code":      statusCode,
		"response_time_ms": responseTimeMs,
	}
	
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		
		result := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			ID:     fmt.Sprintf("%d-0", now.UnixMilli()),
			Values: values,
		})
		
		err := result.Err()
		cancel()
		
		if err == nil {
			if i > 0 {
				log.Printf("Redis write successful after %d retries [%s]", i, targetName)
			}
			log.Printf("Recorded data [%s]: status=%s, response_time=%dms, http=%d", 
				targetName, status, responseTimeMs, statusCode)
			return
		}
		
		if i < maxRetries-1 {
			log.Printf("Redis write failed [%s], retry %d/%d: %v", targetName, i+1, maxRetries-1, err)
			time.Sleep(time.Duration(i+1) * time.Second)
		} else {
			log.Printf("ERROR: Redis write final failure [%s]: %v", targetName, err)
		}
	}
}

func checkTarget(target Target) {
	start := time.Now()
	
	checkMutex.Lock()
	totalChecks++
	checkMutex.Unlock()

	req, err := http.NewRequestWithContext(ctx, "GET", target.URL, nil)
	if err != nil {
		duration := time.Since(start).Milliseconds()
		log.Printf("Request creation failed [%s]: %v", target.Name, err)
		
		checkMutex.Lock()
		failedChecks++
		checkMutex.Unlock()
		
		addRecordToRedis(target.Name, "ERROR", 0, duration)
		return
	}

	req.Header.Set("User-Agent", "Go-Uptime-Monitor/1.0")
	
	resp, err := httpClient.Do(req)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		log.Printf("Check failed [%s]: %s\n", target.Name, err.Error())
		
		checkMutex.Lock()
		failedChecks++
		checkMutex.Unlock()
		
		addRecordToRedis(target.Name, "ERROR", 0, duration)
	} else {
		defer resp.Body.Close()
		status := "UP"
		if resp.StatusCode >= 500 {
			status = "DOWN"
			
			checkMutex.Lock()
			failedChecks++
			checkMutex.Unlock()
		}
		log.Printf("Check successful [%s]: status %s, duration %dms, HTTP code %d\n", target.Name, status, duration, resp.StatusCode)
		addRecordToRedis(target.Name, status, resp.StatusCode, duration)
	}
}

func runCheckers() {
	jobs := make(chan Target, len(targets))
	var wg sync.WaitGroup
	
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	for w := 1; w <= config.WorkerCount; w++ {
		go func(id int, jobs <-chan Target) {
			for {
				select {
				case t, ok := <-jobs:
					if !ok {
						return
					}
					log.Printf("Worker %d: checking %s", id, t.Name)
					checkTarget(t)
					wg.Done()
				case <-workerCtx.Done():
					return
				}
			}
		}(w, jobs)
	}

	dispatchJobs := func() {
		targetsMutex.RLock()
		targetsCopy := make([]Target, len(targets))
		copy(targetsCopy, targets)
		targetsMutex.RUnlock()
		
		log.Printf("Starting to dispatch %d check tasks...", len(targetsCopy))
		
		for _, t := range targetsCopy {
			wg.Add(1)
			select {
			case jobs <- t:
			case <-workerCtx.Done():
				wg.Done()
				return
			}
		}
		wg.Wait()
		log.Println("Current round of check tasks dispatched and completed.")
	}

	dispatchJobs()

	ticker := time.NewTicker(config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dispatchJobs()
		case <-workerCtx.Done():
			return
		}
	}
}

func runAggregator() {
	time.Sleep(10 * time.Second)
	ticker := time.NewTicker(config.AggInterval)
	defer ticker.Stop()

	log.Println("Starting initial data aggregation task...")
	
	targetsMutex.RLock()
	targetsCopy := make([]Target, len(targets))
	copy(targetsCopy, targets)
	targetsMutex.RUnlock()
	
	for _, target := range targetsCopy {
		aggregateForTarget(target)
	}

	for range ticker.C {
		log.Println("Starting data aggregation task...")
		
		targetsMutex.RLock()
		targetsCopy := make([]Target, len(targets))
		copy(targetsCopy, targets)
		targetsMutex.RUnlock()
		
		for _, target := range targetsCopy {
			aggregateForTarget(target)
		}
	}
}

func aggregateForTarget(target Target) {
	streamKey := fmt.Sprintf("uptime:raw:%s", target.Name)
	now := time.Now().UTC()
	oneDayAgo := now.Add(-24 * time.Hour).UnixMilli()

	messages, err := rdb.XRange(ctx, streamKey, fmt.Sprintf("%d-0", oneDayAgo), "+").Result()
	if err != nil || len(messages) == 0 {
		return
	}

	var totalResponseTime, downCount int64 = 0, 0
	checkCount := int64(len(messages))

	for _, msg := range messages {
		responseTime, _ := strconv.ParseInt(msg.Values["response_time_ms"].(string), 10, 64)
		totalResponseTime += responseTime
		if msg.Values["status"].(string) == "DOWN" {
			downCount++
		}
	}

	pipe := rdb.Pipeline()

	dayKey := fmt.Sprintf("uptime:agg:day:%s:%s", target.Name, now.Format("2006-01-02"))

	currentDayStats, _ := rdb.HGetAll(ctx, dayKey).Result()

	var currentTotalChecks, currentTotalResponseTime int64
	if len(currentDayStats) > 0 {
		currentTotalChecks, _ = strconv.ParseInt(currentDayStats["total_checks"], 10, 64)
		currentAvgResponseTime, _ := strconv.ParseInt(currentDayStats["avg_response_time_ms"], 10, 64)
		currentTotalResponseTime = currentAvgResponseTime * currentTotalChecks
	}

	newTotalChecks := currentTotalChecks + checkCount
	newTotalResponseTime := currentTotalResponseTime + totalResponseTime
	var newAvgResponseTime int64
	if newTotalChecks > 0 {
		newAvgResponseTime = newTotalResponseTime / newTotalChecks
	}

	pipe.HSet(ctx, dayKey, "total_checks", newTotalChecks)
	pipe.HIncrBy(ctx, dayKey, "down_count", downCount)
	pipe.HSet(ctx, dayKey, "avg_response_time_ms", newAvgResponseTime)
	pipe.Expire(ctx, dayKey, 90*24*time.Hour)

	_, err = pipe.Exec(ctx)
	if err != nil {
		log.Printf("Aggregation error: failed to write hashes [%s]: %v\n", target.Name, err)
	}

	twoDaysAgo := now.Add(-48 * time.Hour).UnixMilli()
	err = rdb.XTrimMinID(ctx, streamKey, fmt.Sprintf("%d", twoDaysAgo)).Err()
	if err != nil {
		log.Printf("Aggregation error: failed to clean stream [%s]: %v\n", target.Name, err)
	}

	log.Printf("Aggregation complete [%s]: processed %d records\n", target.Name, checkCount)
}

type HealthStatus struct {
	Status       string            `json:"status"`
	Timestamp    string            `json:"timestamp"`
	RedisStatus  string            `json:"redis_status"`
	TargetCount  int               `json:"target_count"`
	WorkerCount  int               `json:"worker_count"`
	Uptime       string            `json:"uptime"`
	Version      string            `json:"version"`
	Config       map[string]string `json:"config"`
}

type Metrics struct {
	TotalChecks      int64             `json:"total_checks"`
	FailedChecks     int64             `json:"failed_checks"`
	AverageResponse  int64             `json:"average_response_ms"`
	ServiceStatus    map[string]string `json:"service_status"`
	LastCheckTime    map[string]string `json:"last_check_time"`
	ResponseTimes    map[string]int64  `json:"response_times_ms"`
}

var (
	startTime    = time.Now()
	totalChecks  int64
	failedChecks int64
	checkMutex   sync.RWMutex
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	redisStatus := "healthy"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	if err := rdb.Ping(ctx).Err(); err != nil {
		redisStatus = "unhealthy: " + err.Error()
	}
	
	targetsMutex.RLock()
	targetCount := len(targets)
	targetsMutex.RUnlock()
	
	health := HealthStatus{
		Status:      "healthy",
		Timestamp:   time.Now().Format(time.RFC3339),
		RedisStatus: redisStatus,
		TargetCount: targetCount,
		WorkerCount: config.WorkerCount,
		Uptime:      time.Since(startTime).String(),
		Version:     "1.0.0",
		Config: map[string]string{
			"check_interval": config.CheckInterval.String(),
			"agg_interval":   config.AggInterval.String(),
			"redis_addr":     config.RedisAddr,
			"port":           config.Port,
		},
	}
	
	if redisStatus != "healthy" {
		health.Status = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	json.NewEncoder(w).Encode(health)
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	targetsMutex.RLock()
	targetsCopy := make([]Target, len(targets))
	copy(targetsCopy, targets)
	targetsMutex.RUnlock()
	
	metrics := Metrics{
		ServiceStatus:  make(map[string]string),
		LastCheckTime:  make(map[string]string),
		ResponseTimes:  make(map[string]int64),
	}
	
	checkMutex.RLock()
	metrics.TotalChecks = totalChecks
	metrics.FailedChecks = failedChecks
	checkMutex.RUnlock()
	
	var totalResponseTime int64
	var responseCount int64
	
	for _, target := range targetsCopy {
		streamKey := fmt.Sprintf("uptime:raw:%s", target.Name)
		
		messages, err := rdb.XRevRange(ctx, streamKey, "+", "-").Result()
		if err == nil && len(messages) > 0 {
			latest := messages[0]
			metrics.ServiceStatus[target.Name] = latest.Values["status"].(string)
			
			timestamp, _ := strconv.ParseInt(strings.Split(latest.ID, "-")[0], 10, 64)
			metrics.LastCheckTime[target.Name] = time.UnixMilli(timestamp).Format(time.RFC3339)
			
			respTime, _ := strconv.ParseInt(latest.Values["response_time_ms"].(string), 10, 64)
			metrics.ResponseTimes[target.Name] = respTime
			
			totalResponseTime += respTime
			responseCount++
		} else {
			metrics.ServiceStatus[target.Name] = "NO_DATA"
			metrics.LastCheckTime[target.Name] = "N/A"
			metrics.ResponseTimes[target.Name] = 0
		}
	}
	
	if responseCount > 0 {
		metrics.AverageResponse = totalResponseTime / responseCount
	}
	
	json.NewEncoder(w).Encode(metrics)
}

func servicesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	targetsMutex.RLock()
	targetsCopy := make([]Target, len(targets))
	copy(targetsCopy, targets)
	targetsMutex.RUnlock()
	
	json.NewEncoder(w).Encode(targetsCopy)
}

func statusApiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "1h"
	}

	targetsMutex.RLock()
	targetsCopy := make([]Target, len(targets))
	copy(targetsCopy, targets)
	targetsMutex.RUnlock()

	allData := make(map[string]interface{})
	for _, target := range targetsCopy {
		data, err := getStatusForService(target.Name, window)
		if err != nil {
			log.Printf("Failed to query service [%s] data: %v", target.Name, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Unable to get data for service '%s'", target.Name),
			})
			return
		}
		allData[target.Name] = data
	}

	json.NewEncoder(w).Encode(allData)
}

func getStatusForService(serviceName, window string) (interface{}, error) {
	switch {
	case strings.HasSuffix(window, "h"):
		hours, _ := strconv.Atoi(strings.TrimSuffix(window, "h"))
		if hours == 0 {
			hours = 1
		}
		now := time.Now()
		start := now.Add(-time.Duration(hours) * time.Hour)
		streamKey := fmt.Sprintf("uptime:raw:%s", serviceName)
		startMs := start.UnixMilli()
		
		log.Printf("Getting service [%s] data: time_window=%s, start_time=%s, timestamp=%d", 
			serviceName, window, start.Format("2006-01-02 15:04:05"), startMs)
		
		result, err := rdb.XRange(ctx, streamKey, fmt.Sprintf("%d-0", startMs), "+").Result()
		if err != nil {
			log.Printf("Failed to get service [%s] data: %v", serviceName, err)
			return nil, err
		}
		log.Printf("Retrieved data points for service [%s]: %d", serviceName, len(result))
		return result, nil

	case strings.HasSuffix(window, "d"):
		days, _ := strconv.Atoi(strings.TrimSuffix(window, "d"))
		if days == 0 {
			days = 7
		}
		results := make(map[string]map[string]string)
		for i := 0; i < days; i++ {
			date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
			dayKey := fmt.Sprintf("uptime:agg:day:%s:%s", serviceName, date)
			res, errHash := rdb.HGetAll(ctx, dayKey).Result()
			if errHash == nil && len(res) > 0 {
				results[date] = res
			}
		}
		return results, nil

	default:
		start := time.Now().Add(-1 * time.Hour).UnixMilli()
		streamKey := fmt.Sprintf("uptime:raw:%s", serviceName)
		return rdb.XRange(ctx, streamKey, fmt.Sprintf("%d-0", start), "+").Result()
	}
}

func initConfig() {
	config = Config{
		RedisAddr:      getEnvOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword:  os.Getenv("REDIS_PASSWORD"),
		RedisDB:        getEnvAsIntOrDefault("REDIS_DB", 0),
		Port:           getEnvOrDefault("PORT", "8080"),
		WorkerCount:    getEnvAsIntOrDefault("WORKER_COUNT", 5),
		CheckInterval:  time.Duration(getEnvAsIntOrDefault("CHECK_INTERVAL_SECONDS", 60)) * time.Second,
		AggInterval:    time.Duration(getEnvAsIntOrDefault("AGG_INTERVAL_MINUTES", 5)) * time.Minute,
		RequestTimeout: time.Duration(getEnvAsIntOrDefault("REQUEST_TIMEOUT_SECONDS", 15)) * time.Second,
	}
	
	httpClient.Timeout = config.RequestTimeout
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func initRedis() error {
	rdb = redis.NewClient(&redis.Options{
		Addr:         config.RedisAddr,
		Password:     config.RedisPassword,
		DB:           config.RedisDB,
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	
	log.Printf("Successfully connected to Redis (%s)", config.RedisAddr)
	return nil
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	initConfig()

	loadConfig()

	if err := initRedis(); err != nil {
		log.Fatalf("Redis initialization failed: %v", err)
	}

	go runCheckers()
	go runAggregator()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/services", servicesHandler)
	mux.HandleFunc("/api/status", statusApiHandler)
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/metrics", metricsHandler)
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	server := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Server starting on http://localhost:%s", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server startup failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctxShutdown); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	if err := rdb.Close(); err != nil {
		log.Printf("Failed to close Redis connection: %v", err)
	}

	log.Println("Server has been gracefully shut down")
}