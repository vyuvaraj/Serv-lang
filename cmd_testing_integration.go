package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// integrationService defines a service to start for integration tests.
type integrationService struct {
	Name   string
	Binary string
	Port   int
	Args   []string
	Env    []string
}

// runIntegrationTests starts real infrastructure services, runs tests, then stops them.
func runIntegrationTests(srvFile string, withCoverage bool, filter string) {
	fmt.Println()
	fmt.Println("  ▲ Integration Test Mode")
	fmt.Println("  ━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Create temp data directory
	tmpDir := ".servtest"
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	// Find available ports
	storePort := findFreePort()
	queuePort := findFreePort()
	cachePort := findFreePort()

	services := []integrationService{
		{
			Name:   "ServStore",
			Binary: "servstore",
			Port:   storePort,
			Args:   []string{"--port", fmt.Sprintf("%d", storePort), "--data-dir", tmpDir + "/store"},
		},
		{
			Name:   "ServQueue",
			Binary: "servqueue",
			Port:   queuePort,
		},
		{
			Name:   "ServCache",
			Binary: "servcache",
			Port:   cachePort,
		},
	}

	// Start services
	var procs []*exec.Cmd
	for _, svc := range services {
		_, err := exec.LookPath(svc.Binary)
		if err != nil {
			fmt.Printf("  ⚠  %s not found on PATH. Skipping.\n", svc.Binary)
			continue
		}

		cmd := exec.Command(svc.Binary, svc.Args...)
		cmd.Env = append(os.Environ(), svc.Env...)
		cmd.Env = append(cmd.Env, fmt.Sprintf("PORT=%d", svc.Port))
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Start(); err != nil {
			fmt.Printf("  ✗  Failed to start %s: %v\n", svc.Name, err)
			continue
		}
		procs = append(procs, cmd)
		fmt.Printf("  ✓  %s started on :%d\n", svc.Name, svc.Port)
	}

	// Wait for services to be healthy
	fmt.Print("  Waiting for services...")
	time.Sleep(2 * time.Second)
	healthy := 0
	for _, svc := range services {
		if pingHealth(svc.Port) {
			healthy++
		}
	}
	fmt.Printf(" %d/%d healthy\n\n", healthy, len(services))

	// Set environment for the test runner
	os.Setenv("SERV_STORE_ENDPOINT", fmt.Sprintf("http://localhost:%d", storePort))
	os.Setenv("SERV_QUEUE_ENDPOINT", fmt.Sprintf("localhost:%d", queuePort))
	os.Setenv("SERV_CACHE_ENDPOINT", fmt.Sprintf("http://localhost:%d", cachePort))
	os.Setenv("SERV_INTEGRATION_TEST", "true")

	// Run the actual tests
	fmt.Println("  Running tests against live infrastructure...")
	fmt.Println("  ━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	runTests(srvFile, withCoverage, filter)

	// Cleanup
	fmt.Println()
	fmt.Println("  Stopping integration services...")
	for _, cmd := range procs {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}
	fmt.Println("  ✓  All services stopped.")
}

func findFreePort() int {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 9100 + int(time.Now().UnixNano()%100)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

func pingHealth(port int) bool {
	client := http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/healthz", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}
