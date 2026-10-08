// Command machine-read-models emits public schemas or synthetic examples from
// Core semantic result types. It performs no runtime operations.
package main

import (
	"flag"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
	"os"
)

func main() {
	schema := flag.Bool("schema", false, "emit schema instead of synthetic examples")
	flag.Parse()
	generate := machinereadmodels.Golden
	if *schema {
		generate = machinereadmodels.Schema
	}
	data, err := generate()
	if err != nil {
		panic(err)
	}
	if _, err = os.Stdout.Write(append(data, '\n')); err != nil {
		panic(err)
	}
}
