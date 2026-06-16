//go:build wasm

package runtime

// Workflow stubs for the WASM target.
// Workflows require SQLite persistence which is not available inside a browser
// sandbox, so these functions are no-ops when targeting WASM.

type WorkflowFn func(ctx *WorkflowCtx, param interface{}) interface{}

type WorkflowCtx struct {
	InstanceID string
	stepIndex  int
}

func (ctx *WorkflowCtx) Step(fn func() interface{}) interface{} {
	// In WASM, steps are never replayed — execute directly.
	return fn()
}

func RegisterWorkflow(name string, fn WorkflowFn) {}

func StartWorkflow(name interface{}, param interface{}) interface{} {
	return nil
}

var WorkflowNoop = struct{}{}
