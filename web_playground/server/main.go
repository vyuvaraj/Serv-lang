package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type RunRequest struct {
	Source string `json:"source"`
}

type RunResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

var (
	servExePath string
	repoRoot    string
)

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "main.go")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

func init() {
	repoRoot = findRepoRoot()
	candidates := []string{
		filepath.Join(repoRoot, "serv.exe"),
		filepath.Join(repoRoot, "serv"),
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err == nil {
			if _, err := os.Stat(abs); err == nil {
				servExePath = abs
				break
			}
		}
	}

	if servExePath == "" {
		log.Println("Warning: serv.exe not found. Execution fallback will run 'go run main.go'")
	} else {
		log.Printf("Using Serv compiler binary: %s\n", servExePath)
	}
}

func main() {
	port := flag.Int("port", 8080, "Port to run the playground server on")
	flag.Parse()

	// Serve static files
	fs := http.FileServer(http.Dir("./web_playground/ui"))
	http.Handle("/", fs)

	// API run endpoint
	http.HandleFunc("/api/run", handleRun)

	log.Printf("Serv Playground Server started on http://localhost:%d\n", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}


func handleRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(RunResponse{Success: false, Error: "Method not allowed"})
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RunResponse{Success: false, Error: "Invalid JSON request"})
		return
	}

	// Create a temporary sandbox directory for this run
	randBytes := make([]byte, 8)
	rand.Read(randBytes)
	runId := hex.EncodeToString(randBytes)

	tempDir := filepath.Join(os.TempDir(), "serv_playground_"+runId)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(RunResponse{Success: false, Error: "Failed to create sandbox"})
		return
	}
	defer os.RemoveAll(tempDir)

	sourceFile := filepath.Join(tempDir, "main.srv")
	if err := os.WriteFile(sourceFile, []byte(req.Source), 0644); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(RunResponse{Success: false, Error: "Failed to write source file"})
		return
	}

	// Step 1: Compile the srv file to a native executable
	binName := "service.exe"
	binPath := filepath.Join(tempDir, binName)

	var compileCmd *exec.Cmd
	if servExePath != "" {
		compileCmd = exec.Command(servExePath, "build", sourceFile, "-o", binName)
	} else {
		mainGo := filepath.Join(repoRoot, "main.go")
		compileCmd = exec.Command("go", "run", mainGo, "build", sourceFile, "-o", binName)
		compileCmd.Dir = repoRoot
	}

	var compileBuf bytes.Buffer
	compileCmd.Stdout = &compileBuf
	compileCmd.Stderr = &compileBuf

	if err := compileCmd.Run(); err != nil {
		json.NewEncoder(w).Encode(RunResponse{
			Success: false,
			Output:  compileBuf.String(),
			Error:   "Compilation failed: " + err.Error(),
		})
		return
	}

	// Step 2: Execute the compiled native binary directly
	runCmd := exec.Command(binPath)
	var runBuf bytes.Buffer
	runCmd.Stdout = &runBuf
	runCmd.Stderr = &runBuf

	if err := runCmd.Start(); err != nil {
		json.NewEncoder(w).Encode(RunResponse{
			Success: false,
			Error:   "Failed to start compiled service: " + err.Error(),
		})
		return
	}

	done := make(chan error, 1)
	go func() {
		done <- runCmd.Wait()
	}()

	var runErr error
	success := false
	select {
	case err := <-done:
		runErr = err
		success = (err == nil)
	case <-time.After(1500 * time.Millisecond):
		// Kill the compiled service process directly
		runCmd.Process.Kill()
		<-done
		success = true
	}

	errStr := ""
	if runErr != nil && !success {
		errStr = runErr.Error()
	}

	resp := RunResponse{
		Success: success,
		Output:  runBuf.String(),
		Error:   errStr,
	}

	json.NewEncoder(w).Encode(resp)
}

