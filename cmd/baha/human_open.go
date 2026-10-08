package main

import (
 "context"
 "fmt"
 "io"
 "net/url"
 "os"
 "os/exec"
 "runtime"
 "strings"

 "github.com/mcpdev80/baseharbor/internal/application"
 "github.com/mcpdev80/baseharbor/internal/cli"
)

func humanOpenCommand(store application.Store) *cli.Command {
 return &cli.Command{
  Name: "open",
  Summary: "Open a verified HTTPS application endpoint",
  Usage: "baha open [--print] [--json]",
  Long: "Uses the selected repository application and its verified TLS deployment state; never invents a URL or opens an unverified endpoint. --print only emits the URL.",
  Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
   printOnly, jsonOutput := false, false
   for _, arg := range args {
    switch arg {
    case "--print": printOnly=true
    case "--json": jsonOutput=true
    default: return usageError("unknown open option "+arg,"Use 'baha open --print' or 'baha open --json'.")
    }
   }
   resolved, err := resolveApplication(ctx,store,nil,"open")
   if err != nil { return err }
   if err := authorizeApplicationOperation(ctx,"app.show",resolved);err!=nil { return err }
   endpoint,err:=verifiedApplicationOpenURL(resolved)
   if err!=nil { return err }
   if jsonOutput { return writeJSON(out,map[string]string{"url":endpoint}) }
   if printOnly { fmt.Fprintln(out,endpoint);return nil }
   if err:=launchApplicationURL(ctx,endpoint);err!=nil {
    return usageError("unable to launch the system browser: "+err.Error(),"The verified URL is "+endpoint+". Run 'baha open --print' to copy it.")
   }
   fmt.Fprintln(out,"Opened "+endpoint)
   return nil
  },
 }
}

func verifiedApplicationOpenURL(resolved resolvedApplication) (string,error) {
 if len(resolved.Manifest.Exposures)==0 {
  return "",usageError("application has no declared HTTP exposure","Declare an HTTP exposure and run 'baha up', then retry.")
 }
 _, obs, err:=collectApplicationTLSObservation(resolved)
 if err!=nil || obs==nil || !obs.Healthy || strings.TrimSpace(obs.Hostname)=="" {
  return "",usageError("application has no verified HTTPS deployment endpoint","Run 'baha status' and 'baha doctor' to inspect exposure/TLS readiness.")
 }
 host:=strings.TrimSpace(obs.Hostname)
 if err:=validateRuntimeHostname(host);err!=nil {
  return "",usageError("application deployment hostname is invalid","Review the deployment TLS/hostname configuration.")
 }
 endpoint:=(&url.URL{Scheme:"https",Host:host,Path:"/"}).String()
 return endpoint,nil
}

var launchApplicationURL = func(ctx context.Context, address string) error {
 switch runtime.GOOS {
 case "linux":
  if _,err:=exec.LookPath("xdg-open");err!=nil{return err}
  return exec.CommandContext(ctx,"xdg-open",address).Run()
 case "darwin":
  return exec.CommandContext(ctx,"open",address).Run()
 default:
  return fmt.Errorf("unsupported host browser launcher on %s",runtime.GOOS)
 }
}

var _ = os.ErrNotExist
