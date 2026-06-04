package runtime

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"strconv"
	"sync"
)

type pythonWorker struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mutex  sync.Mutex
}

// Python interop daemon pool state
var (
	pythonPoolQueue   chan *pythonWorker
	pythonWorkersOnce sync.Once
)

func initPythonDaemonPool() {
	pythonWorkersOnce.Do(func() {
		workersCount := 4
		if valStr := Config("python.workers"); valStr != "" {
			if val, err := strconv.Atoi(valStr); err == nil && val > 0 {
				workersCount = val
			}
		}

		pythonPoolQueue = make(chan *pythonWorker, workersCount)

		for i := 0; i < workersCount; i++ {
			worker := spawnPythonWorker()
			if worker == nil {
				panic("Failed to start Python worker during pool initialization")
			}
			pythonPoolQueue <- worker
		}
	})
}

// Call Python Script for extern mappings using the persistent daemon pool
func CallPython(scriptPath string, funcName string, args ...interface{}) interface{} {
	endSpan := TraceExtern("python:"+scriptPath, funcName)
	defer endSpan()

	initPythonDaemonPool()

	worker := <-pythonPoolQueue
	defer func() {
		pythonPoolQueue <- worker
	}()

	worker.mutex.Lock()
	defer worker.mutex.Unlock()

	// Health check: if the process has exited, respawn it
	if worker.cmd.ProcessState != nil || !isProcessAlive(worker.cmd) {
		LogWarn("Python worker died, respawning...")
		newWorker := spawnPythonWorker()
		if newWorker != nil {
			worker.cmd = newWorker.cmd
			worker.stdin = newWorker.stdin
			worker.stdout = newWorker.stdout
		} else {
			LogError("Failed to respawn Python worker")
			return nil
		}
	}

	reqPayload := map[string]interface{}{
		"script_path": scriptPath,
		"func_name":   funcName,
		"args":        args,
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		LogError("Failed to marshal Python daemon request: ", err)
		return nil
	}

	_, err = worker.stdin.Write(append(payloadBytes, '\n'))
	if err != nil {
		// Write failed — worker is likely dead, try respawn once
		LogWarn("Python worker write failed, respawning: ", err)
		newWorker := spawnPythonWorker()
		if newWorker == nil {
			LogError("Failed to respawn Python worker after write error")
			return nil
		}
		worker.cmd = newWorker.cmd
		worker.stdin = newWorker.stdin
		worker.stdout = newWorker.stdout

		// Retry the write
		_, err = worker.stdin.Write(append(payloadBytes, '\n'))
		if err != nil {
			LogError("Failed to write to respawned Python worker: ", err)
			return nil
		}
	}

	line, err := worker.stdout.ReadBytes('\n')
	if err != nil {
		LogError("Failed to read response from Python daemon: ", err)
		return nil
	}

	var res struct {
		Result interface{} `json:"result"`
		Error  string      `json:"error"`
	}

	if err := json.Unmarshal(line, &res); err != nil {
		LogError("Failed to unmarshal Python daemon response: ", err)
		return string(line)
	}

	if res.Error != "" {
		LogError("Python daemon execution error: ", res.Error)
		return nil
	}

	return res.Result
}

// isProcessAlive checks if the underlying process is still running.
func isProcessAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	// On Unix, sending signal 0 checks if process exists.
	// On Windows, Process.Signal is not fully supported, so we check ProcessState.
	if cmd.ProcessState != nil {
		return false
	}
	return true
}

// spawnPythonWorker creates a single new Python daemon worker.
func spawnPythonWorker() *pythonWorker {
	daemonCode := `
import sys
import json
import importlib.util

modules = {}

while True:
    line = sys.stdin.readline()
    if not line:
        break
    try:
        req = json.loads(line)
        script_path = req["script_path"]
        func_name = req["func_name"]
        args = req["args"]

        if script_path not in modules:
            spec = importlib.util.spec_from_file_location("module", script_path)
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
            modules[script_path] = module
        else:
            module = modules[script_path]

        fn = getattr(module, func_name)
        res = fn(*args)
        print(json.dumps({"result": res}))
        sys.stdout.flush()
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.stdout.flush()
`
	cmd := exec.Command("python", "-u", "-c", daemonCode)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		LogError("Failed to create stdin pipe for Python worker: ", err)
		return nil
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		LogError("Failed to create stdout pipe for Python worker: ", err)
		return nil
	}
	stdout := bufio.NewReader(stdoutPipe)

	if err := cmd.Start(); err != nil {
		LogError("Failed to start Python worker: ", err)
		return nil
	}

	return &pythonWorker{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}
}

// shutdownPythonWorkers terminates all Python daemon workers.
func shutdownPythonWorkers() {
	if pythonPoolQueue == nil {
		return
	}
	// Drain the pool and kill each worker
	for {
		select {
		case worker := <-pythonPoolQueue:
			if worker.stdin != nil {
				worker.stdin.Close()
			}
			if worker.cmd != nil && worker.cmd.Process != nil {
				worker.cmd.Process.Kill()
				worker.cmd.Wait()
			}
		default:
			return
		}
	}
}


