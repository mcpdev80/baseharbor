package main

import (
 "context"
 "fmt"
 "io"
 "sort"

 "github.com/mcpdev80/baseharbor/internal/application"
 "github.com/mcpdev80/baseharbor/internal/cli"
)

// appCapabilityCommand exposes provider-neutral, read-only capability discovery.
// Terminal access is offered only by providers with a real execution contract.
// It must not fabricate a shell or reveal credentials.
func appCapabilityCommand(store application.Store, capability string) *cli.Command {
 return &cli.Command{
  Name: capability,
  Summary: "Inspect the application's "+capability+" capability",
  Usage: "baha app "+capability+" [--app NAME] [--json]",
  Long: "Shows configured capability instances; use the provider's supported typed commands for actions. Does not expose secrets or invent unsupported operations.",
  Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
   filtered, format, err := parseReadOutputArgs(args, "app "+capability)
   if err != nil { return err }
   appName, instance, err := parseAccessTarget(filtered, capability)
   if err != nil { return err }
   if instance != "" { return usageError("instance argument is not supported by capability overview", "Omit INSTANCE to list declared instances, then choose a supported provider action.") }
   var appArgs []string
   if appName!="" { appArgs=[]string{appName} }
   resolved, err := resolveApplication(ctx, store, appArgs, capability)
   if err != nil { return err }
   if err:=authorizeApplicationOperation(ctx,"app.show",resolved);err!=nil{return err}
   instances, provider, supported := applicationCapabilityInstances(resolved.Manifest,capability)
   if !supported { return usageError("unsupported capability "+capability,"Use baha app show to inspect supported application requirements.") }
   sort.Strings(instances)
   result:=struct {
    Capability string `json:"capability"`
    Application string `json:"application"`
    Target string `json:"target"`
    Provider string `json:"provider"`
    Instances []string `json:"instances"`
    Mode string `json:"mode"`
   }{Capability:capability,Application:resolved.Manifest.Name,Target:resolved.Target.Name,Provider:provider,Instances:instances,Mode:"read-only"}
   if instances==nil { result.Instances=[]string{} }
   if format==outputJSON {return writeJSON(out,result)}
   fmt.Fprintf(out,"%s for %s (target %s)\nProvider: %s\n",capability,result.Application,result.Target,provider)
   if len(instances)==0 { fmt.Fprintln(out,"Not configured for this application.");return nil }
   for _,name:=range instances {fmt.Fprintf(out,"  %s\n",name)}
   fmt.Fprintln(out,"Actions: inspect with baha app show or baha app doctor; interactive access is available only when supported by the provider.")
   return nil
  },
 }
}

func applicationCapabilityInstances(m application.Manifest,capability string)([]string,string,bool){
 switch capability {
 case "document-db":return application.DocumentDatabaseInstanceNames(m),"mongodb",true
 case "queue":return application.MessagingQueueInstanceNames(m),"rabbitmq",true
 case "pubsub":return application.MessagingPubSubInstanceNames(m),"rabbitmq",true
 case "stream":return application.MessagingStreamInstanceNames(m),"rabbitmq",true
 case "storage":return application.ObjectStorageBucketNames(m),"s3-compatible",true
 case "identity":
  if application.HasIdentity(m){return []string{"default"},"oidc",true}
  return nil,"oidc",true
 default:return nil,"",false
 }
}
