package main

import (
 "bufio"
 "context"
 "fmt"
 "io"
 "sort"
 "strconv"
 "strings"

 "github.com/mcpdev80/baseharbor/internal/deployment"
)

func guidedTargetActivation(ctx context.Context, out, errOut io.Writer) error {
 if noInput(ctx) || !readerIsTerminal(appNewInput) {
  return usageError("target activate needs a name in non-interactive mode","Run baha target list and then baha target activate NAME.")
 }
 cfg,err:=deployment.LoadConfig()
 if err!=nil{return err}
 names:=cfg.TargetNames()
 if _,ok:=cfg.Targets["local"];!ok{names=append(names,"local")}
 sort.Strings(names)
 if len(names)==0{return usageError("no target available","Run baha new target to create one.")}
 fmt.Fprintln(out,"Select a deployment Target:")
 for i,name:=range names {fmt.Fprintf(out,"  %d  %s\n",i+1,name)}
 fmt.Fprint(out,"Choice (or cancel): ")
 line,err:=bufio.NewReader(appNewInput).ReadString('\n')
 if err!=nil{return fmt.Errorf("read target selection: %w",err)}
 choice:=strings.TrimSpace(strings.ToLower(line))
 if choice=="cancel"||choice=="q" {fmt.Fprintln(out,"Cancelled. No changes were made.");return nil}
 index,err:=strconv.Atoi(choice)
 if err!=nil||index<1||index>len(names){return usageError("invalid target choice","Select a listed number or type cancel.")}
 if err:=writePersistedTarget(names[index-1]);err!=nil{return err}
 fmt.Fprintf(out,"Active target: %s\n",names[index-1])
 return nil
}
