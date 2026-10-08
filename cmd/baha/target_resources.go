package main

import (
 "context"
 "fmt"
 "io"

 "github.com/mcpdev80/baseharbor/internal/cli"
 "github.com/mcpdev80/baseharbor/internal/machine"
)

type currentDeviceResourcesResult struct {
 ContractVersion string `json:"contract_version"`
 Scope string `json:"scope"`
 Target string `json:"selected_target"`
 SelectionOrigin string `json:"selection_origin"`
 Available bool `json:"available"`
 MemoryTotalKiB uint64 `json:"memory_total_kib,omitempty"`
 MemoryAvailableKiB uint64 `json:"memory_available_kib,omitempty"`
 SwapTotalKiB uint64 `json:"swap_total_kib,omitempty"`
 SwapAvailableKiB uint64 `json:"swap_available_kib,omitempty"`
 UnavailableReason string `json:"unavailable_reason,omitempty"`
}

func currentDeviceResourcesCommand() *cli.Command {
 return &cli.Command{
  Name:"resources",
  Summary:"Inspect this CLI device's memory (never remote target metrics)",
  Usage:"baha target resources [--json]",
  Run:func(ctx context.Context,args []string,out,errOut io.Writer) error {
   filtered,format,err:=parseReadOutputArgs(args,"target resources")
   if err!=nil{return err}
   if len(filtered)>0{return usageError("target resources does not accept arguments","Run 'baha target resources --json' for machine-readable device metrics.")}
   target,err:=effectiveTarget(ctx)
   if err!=nil{return err}
   result:=currentDeviceResourcesResult{ContractVersion:machine.ContractVersion,Scope:"current-device",Target:target.Name,SelectionOrigin:targetSelectionOrigin(ctx)}
   mem,memErr:=readCurrentDeviceMemory()
   if memErr==nil {
    result.Available=true
    result.MemoryTotalKiB=mem.TotalKiB
    result.MemoryAvailableKiB=mem.AvailableKiB
    result.SwapTotalKiB=mem.SwapTotalKiB
    result.SwapAvailableKiB=mem.SwapFreeKiB
   } else {result.UnavailableReason="local host memory telemetry is unsupported or unavailable"}
   if format==outputJSON{return writeJSON(out,result)}
   fmt.Fprint(out,currentDeviceResources())
   fmt.Fprintf(out,"Selected Target: %s (%s)\n",target.Name,result.SelectionOrigin)
   return nil
  },
 }
}
