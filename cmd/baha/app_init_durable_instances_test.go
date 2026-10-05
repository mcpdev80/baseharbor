package main

import (
	"bufio"
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestGuidedDurableInstancesAvoidCacheNameCollisions(t *testing.T) {
	cases := []struct {
		name       string
		cache      bool
		cacheNames []string
		input      string
		want       []string
		reprompt   bool
	}{
		{"single durable default", false, nil, "\n", nil, false},
		{"cache default and durable", true, nil, "\n", []string{"store"}, false},
		{"named cache permits durable default", true, []string{"sessions"}, "\n", nil, false},
		{"occupied suggestion", true, []string{"default", "store", "store-2"}, "\n", []string{"store-3"}, false},
		{"explicit default collision", true, nil, "default\n\n", []string{"store"}, true},
		{"explicit named collision", true, []string{"sessions"}, "sessions\n\n", nil, true},
		{"explicit distinct names", true, nil, "archive,events\n", []string{"archive", "events"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			names, err := promptDurableKeyValueInstances(bufio.NewReader(strings.NewReader(tc.input)), &out, tc.cache, tc.cacheNames)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("names=%v want=%v", names, tc.want)
			}
			if strings.Contains(out.String(), "already used by the cache") != tc.reprompt {
				t.Fatalf("unexpected prompt: %s", out.String())
			}
			selected := make([]bool, guidedCapabilityCount)
			selected[guidedCapabilityCache] = tc.cache
			selected[guidedCapabilityDurableKeyValue] = true
			m, err := buildGuidedInitManifest(bufio.NewReader(strings.NewReader("")), &out, appProjectDetection{}, guidedInitSelection{name: "demo", environment: "dev", selected: selected, cacheInstances: tc.cacheNames, keyValueInstances: names})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Validate(); err != nil {
				t.Fatalf("generated manifest is invalid: %v", err)
			}
			for _, cache := range application.CacheInstanceNames(m) {
				for _, durable := range application.KeyValueInstanceNames(m) {
					if cache == durable {
						t.Fatalf("shared cache/durable identity %q", cache)
					}
				}
			}
		})
	}
}
