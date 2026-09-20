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
		"SECRET_KEY               missing - will be generated automatically baha app apply",
		"OPENAI_API_KEY           missing - user input required          baha app secret set OPENAI_API_KEY --stdin",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}
