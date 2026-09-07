package preflight

import (
	"context"
	"fmt"
	"io"
)

type Check struct {
	Name string
	Run  func(context.Context) error
}

type Result struct {
	Name   string
	OK     bool
	Detail string
}

func Run(ctx context.Context, checks []Check) ([]Result, bool) {
	results := make([]Result, 0, len(checks))
	allOK := true
	for _, check := range checks {
		err := check.Run(ctx)
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
