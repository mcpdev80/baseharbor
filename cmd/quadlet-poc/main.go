package main

import (
	"flag"
	"fmt"
	"os"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func main() {
	var composePath string
	var service string
	var prefix string
	var outDir string

	flag.StringVar(&composePath, "compose", "", "path to compose.yaml")
	flag.StringVar(&service, "service", "", "Compose service to render")
	flag.StringVar(&prefix, "prefix", "baseharbor-poc", "Quadlet unit prefix")
	flag.StringVar(&outDir, "out", "", "output directory")
	flag.Parse()

	if composePath == "" || service == "" || outDir == "" {
		fmt.Fprintln(os.Stderr, "--compose, --service and --out are required")
		os.Exit(2)
	}

	workload, err := bhruntime.RenderComposeServiceQuadlet(composePath, service, prefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := bhruntime.WriteQuadletWorkload(outDir, workload); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("unit=%s\ncontainer=%s\n", workload.ServiceUnit, workload.ContainerName)
}
