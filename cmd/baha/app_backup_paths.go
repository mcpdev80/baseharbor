package main

import (
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
 "time"

 "github.com/mcpdev80/baseharbor/internal/application"
)

// gitWorktreeContaining reports the nearest enclosing Git worktree from the
// current repository, accepting both .git directories and worktree .git files.
func gitWorktreeContaining(path string) bool {
 cwd,err:=os.Getwd()
 if err!=nil{return false}
 root:=""
 for dir:=cwd;;{
  if _,err:=os.Lstat(filepath.Join(dir,".git"));err==nil {root=dir;break}
  parent:=filepath.Dir(dir)
  if parent==dir{break}
  dir=parent
 }
 if root==""{return false}
 absolute,err:=filepath.Abs(path)
 if err!=nil{return false}
 relative,err:=filepath.Rel(root,absolute)
 if err!=nil{return false}
 return relative=="." || (relative!=".." && !strings.HasPrefix(relative,".."+string(os.PathSeparator)))
}

func backupOutputFlag(args []string) string {
 for i,arg:=range args {
  if arg=="--output" && i+1<len(args){return args[i+1]}
  if strings.HasPrefix(arg,"--output="){return strings.TrimPrefix(arg,"--output=")}
 }
 return ""
}

func reportGitBackupRisk(out io.Writer,path string) {
 if path!=""&&gitWorktreeContaining(path) {
  fmt.Fprintf(out,"WARNING: Backup %s is inside a Git worktree. 'git add -A' could commit sensitive archived state. Prefer the default XDG backup directory.\n",path)
 }
}

func prepareNonInteractiveBackupOutput(ctx context.Context,store application.Store,args []string,errOut io.Writer)([]string,error){
 if supplied:=backupOutputFlag(args);supplied!=""{
  reportGitBackupRisk(errOut,supplied)
  return args,nil
 }
 filtered,_,err:=extractRecoverySelectionArgs(args)
 if err!=nil{return nil,err}
 filtered,environment,err:=extractApplicationEnvironment(filtered,"backup")
 if err!=nil{return nil,err}
 withoutPassword:=make([]string,0,len(filtered))
 for i:=0;i<len(filtered);i++ {
  if filtered[i]=="--password-file" {i++;continue}
  if strings.HasPrefix(filtered[i],"--password-file="){continue}
  withoutPassword=append(withoutPassword,filtered[i])
 }
 name,_,err:=parseGuidedBackupArgs(withoutPassword)
 if err!=nil{return nil,err}
 var appArgs []string
 if name!=""{appArgs=[]string{name}}
 resolved,err:=resolveApplicationEnvironment(ctx,store,appArgs,"backup",environment)
 if err!=nil{return nil,err}
 archive,err:=defaultGuidedBackupPath(resolved.Manifest.Name,resolved.Manifest.Environment,time.Now())
 if err!=nil{return nil,err}
 fmt.Fprintf(errOut,"Backup destination: %s (owner-only XDG data; outside the repository)\n",archive)
 return append(append([]string(nil),args...),"--output",archive),nil
}

func ensureDefaultBackupOutsideGit(path string) error {
 if gitWorktreeContaining(path) {
  return errors.New("default backup destination resolves inside the Git worktree; configure XDG_DATA_HOME outside the repository or choose a safe --output")
 }
 return nil
}
