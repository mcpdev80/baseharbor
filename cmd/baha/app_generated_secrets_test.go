package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestGeneratedSecretReadinessIsActionable(t *testing.T) {
	var out bytes.Buffer
	printRequiredSecretStatus(&out, []openbao.RequiredSecretStatus{
		{Name: "SECRET_KEY", Generated: true},
		{Name: "OPENAI_API_KEY"},
	})

	text := out.String()
	for _, want := range []string{
		"SECRET_KEY\tmissing - will be generated automatically\tbaha app apply",
		"OPENAI_API_KEY\tmissing - user input required\tbaha app secret set OPENAI_API_KEY --stdin",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}
