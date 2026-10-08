package main

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"

 "github.com/mcpdev80/baseharbor/internal/deployment"
)

func targetSelectionPath() (string,error) {
 configPath,err:=deployment.ConfigPath()
 if err!=nil{return "",err}
 return filepath.Join(filepath.Dir(configPath),"active-target"),nil
}

func readPersistedTarget() (string,error) {
 path,err:=targetSelectionPath()
 if err!=nil{return "",err}
 data,err:=os.ReadFile(path)
 if errors.Is(err,os.ErrNotExist){return "",nil}
 if err!=nil{return "",fmt.Errorf("read active target: %w",err)}
 name:=strings.TrimSpace(string(data))
 if err:=deployment.ValidateTargetName(name);err!=nil{return "",fmt.Errorf("invalid persisted target: %w",err)}
 return name,nil
}

func writePersistedTarget(name string) error {
 if err:=deployment.ValidateTargetName(name);err!=nil{return err}
 path,err:=targetSelectionPath()
 if err!=nil{return err}
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(path),".active-target-*")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 if err:=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err:=f.WriteString(name+"\n");err!=nil{f.Close();return err}
 if err:=f.Sync();err!=nil{f.Close();return err}
 if err:=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),path)
}

func clearPersistedTarget() error {
 path,err:=targetSelectionPath()
 if err!=nil{return err}
 err=os.Remove(path)
 if errors.Is(err,os.ErrNotExist){return nil}
 return err
}

func selectedTargetName(explicit,environment string,cfg deployment.Config) (string,error) {
 if explicit!="" {return explicit,nil}
 if environment!="" {return environment,nil}
 persisted,err:=readPersistedTarget()
 if err!=nil{return "",err}
 if persisted!="" {
  if _,ok:=cfg.Targets[persisted];!ok{return "",fmt.Errorf("activated target %q no longer exists; run 'baha target deactivate' and select a valid target",persisted)}
  return persisted,nil
 }
 if cfg.DefaultTarget!="" {return cfg.DefaultTarget,nil}
 if len(cfg.Targets)==1 {for name:=range cfg.Targets{return name,nil}}
 if len(cfg.Targets)>1 {return "",fmt.Errorf("multiple deployment targets configured: run 'baha target activate NAME' or provide --target NAME")}
 return "",nil
}
