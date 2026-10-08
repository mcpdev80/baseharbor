package main

import (
 "bufio"
 "fmt"
 "os"
 "strconv"
 "strings"
)

type localHostMemory struct {
 TotalKiB uint64
 AvailableKiB uint64
 SwapTotalKiB uint64
 SwapFreeKiB uint64
}

func readCurrentDeviceMemory() (localHostMemory,error) {
 f,err:=os.Open("/proc/meminfo")
 if err!=nil {return localHostMemory{},err}
 defer f.Close()
 var mem localHostMemory
 scanner:=bufio.NewScanner(f)
 for scanner.Scan() {
  parts:=strings.Fields(scanner.Text())
  if len(parts)<2 {continue}
  val,err:=strconv.ParseUint(parts[1],10,64)
  if err!=nil{return localHostMemory{},fmt.Errorf("parse current-device memory: %w",err)}
  switch parts[0] {
  case "MemTotal:":mem.TotalKiB=val
  case "MemAvailable:":mem.AvailableKiB=val
  case "SwapTotal:":mem.SwapTotalKiB=val
  case "SwapFree:":mem.SwapFreeKiB=val
  }
 }
 if err:=scanner.Err();err!=nil{return localHostMemory{},err}
 if mem.TotalKiB==0{return localHostMemory{},fmt.Errorf("current-device memory telemetry is unavailable")}
 return mem,nil
}

func currentDeviceResources() string {
 mem,err:=readCurrentDeviceMemory()
 if err!=nil {return "Current Device (CLI host)\n  Resource telemetry unavailable on this platform\n"}
 return fmt.Sprintf("Current Device (CLI host, not a deployment Target)\n  RAM total        %.1f GiB\n  RAM available    %.1f GiB\n  Swap total       %.1f GiB\n  Swap available   %.1f GiB\n", float64(mem.TotalKiB)/1048576,float64(mem.AvailableKiB)/1048576,float64(mem.SwapTotalKiB)/1048576,float64(mem.SwapFreeKiB)/1048576)
}
