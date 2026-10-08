package main
import (
 "bytes"
 "context"
 "encoding/json"
 "testing"
 "github.com/mcpdev80/baseharbor/internal/application"
)

func TestTUINonTTYJSONFallbackDoesNotRequireTerminal(t *testing.T) {
 configureTestTarget(t)
 var out,errOut bytes.Buffer
 err:=tuiCommand(application.DefaultStore()).Run(context.Background(),[]string{"--json"},&out,&errOut)
 if err!=nil && bytes.Contains([]byte(err.Error()),[]byte("interactive terminal")) {t.Fatalf("JSON fallback unexpectedly requires terminal: %v",err)}
 if err==nil{
  var report map[string]any
  if decodeErr:=json.Unmarshal(out.Bytes(),&report);decodeErr!=nil{t.Fatalf("JSON fallback returned non-JSON: %q: %v",out.String(),decodeErr)}
 }
}
