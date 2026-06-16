//go:build !wasm

package runtime

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	_ "github.com/glebarez/go-sqlite"
)

// ---------------------------------------------------------------------------
// Workflow Registry
// ---------------------------------------------------------------------------

// WorkflowFn is the signature for a compiled workflow function.
// It receives the workflow context (which manages step replay) and the param.
type WorkflowFn func(ctx *WorkflowCtx, param interface{}) interface{}

var (
	workflowRegistry   = make(map[string]WorkflowFn)
	workflowRegistryMu sync.RWMutex
)

// RegisterWorkflow registers a workflow function by name so it can be
// started via workflow.start("Name", param).
func RegisterWorkflow(name string, fn WorkflowFn) {
	workflowRegistryMu.Lock()
	defer workflowRegistryMu.Unlock()
	workflowRegistry[name] = fn
}

// ---------------------------------------------------------------------------
// Persistence (SQLite)
// ---------------------------------------------------------------------------

var (
	wfDB   *sql.DB
	wfDBMu sync.Mutex
)

func openWorkflowDB() (*sql.DB, error) {
	wfDBMu.Lock()
	defer wfDBMu.Unlock()
	if wfDB != nil {
		return wfDB, nil
	}

	// Re-use the application database connection if already open, otherwise
	// open a dedicated workflow store so the feature works standalone.
	dsn := "file:serv_workflows.db?cache=shared&mode=rwc&_journal_mode=WAL"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("workflow: open db: %w", err)
	}
	if _, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS workflow_instances (
			id     TEXT PRIMARY KEY,
			name   TEXT NOT NULL,
			param  TEXT,
			status TEXT NOT NULL DEFAULT 'running'
		);
		CREATE TABLE IF NOT EXISTS workflow_steps (
			instance_id TEXT    NOT NULL,
			step_index  INTEGER NOT NULL,
			result      TEXT,
			PRIMARY KEY (instance_id, step_index)
		);
	`); err != nil {
		return nil, fmt.Errorf("workflow: create tables: %w", err)
	}
	wfDB = db
	return wfDB, nil
}

// ---------------------------------------------------------------------------
// WorkflowCtx — per-execution replay context
// ---------------------------------------------------------------------------

// WorkflowCtx is passed into every workflow execution.  It tracks the current
// step index and caches already-completed step results so they are not
// re-executed on replay.
type WorkflowCtx struct {
	InstanceID string
	stepIndex  int
	db         *sql.DB
}

// Step wraps a single workflow step. If the step result is already stored in
// the database (replay scenario) the stored value is returned immediately
// without calling fn. Otherwise fn is called, its result is persisted, and
// then returned.
func (ctx *WorkflowCtx) Step(fn func() interface{}) interface{} {
	idx := ctx.stepIndex
	ctx.stepIndex++

	// Check for a saved result.
	var raw string
	err := ctx.db.QueryRow(
		`SELECT result FROM workflow_steps WHERE instance_id = ? AND step_index = ?`,
		ctx.InstanceID, idx,
	).Scan(&raw)
	if err == nil {
		// Step was already completed — decode and return the cached result.
		var result interface{}
		if jsonErr := json.Unmarshal([]byte(raw), &result); jsonErr == nil {
			return result
		}
		return raw
	}

	// Execute the step.
	result := fn()

	// Persist the result.
	encoded, _ := json.Marshal(result)
	_, _ = ctx.db.Exec(
		`INSERT OR IGNORE INTO workflow_steps (instance_id, step_index, result) VALUES (?, ?, ?)`,
		ctx.InstanceID, idx, string(encoded),
	)
	return result
}

// ---------------------------------------------------------------------------
// Public API — StartWorkflow
// ---------------------------------------------------------------------------

// StartWorkflow starts a new workflow instance asynchronously.
// name:  the registered workflow name.
// param: the input value passed to the workflow.
// Returns the new instance ID.
func StartWorkflow(name interface{}, param interface{}) interface{} {
	nameStr, ok := name.(string)
	if !ok {
		fmt.Printf("workflow.start: name must be a string, got %T\n", name)
		return nil
	}

	workflowRegistryMu.RLock()
	fn, found := workflowRegistry[nameStr]
	workflowRegistryMu.RUnlock()
	if !found {
		fmt.Printf("workflow.start: unknown workflow %q\n", nameStr)
		return nil
	}

	db, err := openWorkflowDB()
	if err != nil {
		fmt.Printf("workflow.start: %v\n", err)
		return nil
	}

	// Generate a simple unique ID.
	var seq int64
	_ = db.QueryRow(`SELECT COUNT(*) FROM workflow_instances`).Scan(&seq)
	instanceID := fmt.Sprintf("%s-%d", nameStr, seq+1)

	// Persist the encoded param.
	paramJSON, _ := json.Marshal(param)
	_, _ = db.Exec(
		`INSERT OR IGNORE INTO workflow_instances (id, name, param, status) VALUES (?, ?, ?, 'running')`,
		instanceID, nameStr, string(paramJSON),
	)

	ctx := &WorkflowCtx{InstanceID: instanceID, db: db}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("workflow %s panicked: %v\n", instanceID, r)
				_, _ = db.Exec(`UPDATE workflow_instances SET status = 'failed' WHERE id = ?`, instanceID)
			}
		}()
		fn(ctx, param)
		_, _ = db.Exec(`UPDATE workflow_instances SET status = 'completed' WHERE id = ?`, instanceID)
	}()

	return instanceID
}

// WorkflowNoop is a blank-identifier guard so the package is always "used".
var WorkflowNoop = struct{}{}
