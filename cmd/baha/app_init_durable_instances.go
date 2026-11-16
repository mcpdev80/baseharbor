package main

import (
	"bufio"
	"fmt"
	"io"
)

func promptDurableKeyValueInstances(reader *bufio.Reader, out io.Writer, cacheEnabled bool, cacheInstances []string) ([]string, error) {
	cacheNames := map[string]struct{}{}
	if cacheEnabled {
		if len(cacheInstances) == 0 {
			cacheInstances = []string{"default"}
		}
		for _, name := range cacheInstances {
			cacheNames[name] = struct{}{}
		}
	}
	fallback := "default"
	if _, collision := cacheNames[fallback]; collision {
		fallback = "store"
		for ordinal := 2; ; ordinal++ {
			if _, collision := cacheNames[fallback]; !collision {
				break
			}
			fallback = fmt.Sprintf("store-%d", ordinal)
		}
	}
	for {
		names, err := promptServiceInstancesDefault(reader, out, "Durable Valkey / Redis", nil, fallback)
		if err != nil {
			return nil, err
		}
		effective := names
		if len(effective) == 0 {
			effective = []string{"default"}
		}
		collision := ""
		for _, name := range effective {
			if _, exists := cacheNames[name]; exists {
				collision = name
				break
			}
		}
		if collision == "" {
			return names, nil
		}
		fmt.Fprintf(out, "Instance %q is already used by the cache; choose a different durable database name.\n", collision)
	}
}
