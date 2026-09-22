package preflight

import (
	"context"
	"fmt"
	"io"
	"time"
)

type Check struct {
	Name string
	Run  func(context.Context) error
}

type Result struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

func Run(ctx context.Context, checks []Check) ([]Result, bool) {
	return run(ctx, checks, 0)
}

// RunWithTimeout gives every check its own timeout budget. A single shared
// deadline makes later checks fail only because earlier runtime checks were
// slow, which is especially visible with external Compose providers.
func RunWithTimeout(ctx context.Context, checks []Check, timeout time.Duration) ([]Result, bool) {
	return run(ctx, checks, timeout)
}

func run(ctx context.Context, checks []Check, timeout time.Duration) ([]Result, bool) {
	results := make([]Result, 0, len(checks))
	allOK := true
	for _, check := range checks {
		checkCtx := ctx
		cancel := func() {}
		if timeout > 0 {
			checkCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		err := check.Run(checkCtx)
		cancel()
		result := Result{Name: check.Name, OK: err == nil}
		if err != nil {
			result.Detail = err.Error()
			allOK = false
		}
		results = append(results, result)
	}
	return results, allOK
}

func Format(w io.Writer, results []Result) {
	for _, result := range results {
		status := "OK"
		if !result.OK {
			status = "FAIL"
		}
		if result.Detail == "" {
			fmt.Fprintf(w, "[%s] %s\n", status, result.Name)
		} else {
			fmt.Fprintf(w, "[%s] %s: %s\n", status, result.Name, result.Detail)
		}
	}
}
