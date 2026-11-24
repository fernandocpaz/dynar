package app

import (
 "context"
 "os"
 "path/filepath"
 "fmt"
 "io"
 "sort"
 "strings"
 "time"

 "github.com/fernandocpaz/dynar/internal/update"
 "github.com/fernandocpaz/tailg/pkg/logui"
)

type SharedOptions struct {
 Source, Namespace, Cluster, Service, Search string
 Regex bool
 Since, Refresh time.Duration
 BufferLines int
 NoUpdateCheck bool
 All bool
 SplitPanes bool
 ServiceExact bool
 Level string
 FilterFile string
}

func sharedViewer(ctx context.Context,c Collector,opts SharedOptions,stdout,stderr io.Writer)(bool,error){
 if !opts.NoUpdateCheck && !isUpdateCheckDisabled() && StableBuild(){
  restart,quit,err:=promptDynaRUpdate(ctx)
  if err!=nil{fmt.Fprintln(stderr,"DynaR update check:",err)}
  if quit{return false,nil}
  if restart{return true,nil}
 }
 broker:=NewBroker(c,opts.Source,opts.Namespace,opts.Cluster,opts.Since,opts.Refresh,opts.BufferLines)
 fmt.Fprintf(stderr,"DynaR: loading %s namespace logs from %s...\n",opts.Namespace,opts.Source)
 if err:=broker.Bootstrap(ctx);err!=nil{return false,fmt.Errorf("load namespace: %w",err)}
 if err:=broker.Status();err!=nil{fmt.Fprintln(stderr,"WARNING: Dynatrace returned incomplete history:",err)}
 go broker.Run(ctx)
 for {
  apps:=broker.Apps()
  if len(apps)==0{
   return false,fmt.Errorf("no accessible logs in namespace %q during %s; check ingestion, scope, and --since",opts.Namespace,opts.Since)
  }
  var selected map[string]bool
  var record bool
  var recordFrom time.Time
  switch {
  case opts.All:selected=map[string]bool{}
  case opts.Service!="":
   selected=make(map[string]bool)
   for _,app:=range apps{
    match:=strings.Contains(strings.ToLower(app.Name),strings.ToLower(opts.Service))
    if opts.ServiceExact{match=strings.EqualFold(app.Name,opts.Service)}
    if match{selected[app.Name]=true}
   }
   if len(selected)==0{return false,fmt.Errorf("no matching workload for --service %q",opts.Service)}
  default:
   choice,err:=logui.PickApps(apps,opts.Source,opts.Namespace)
   if err!=nil{return false,err}
   if choice.Quit{return false,nil}
   if len(choice.Workloads)==0{continue}
   record=choice.Record
   recordFrom=choice.RecordFrom
   selected=make(map[string]bool,len(choice.Workloads))
   for _,app:=range choice.Workloads{selected[app]=true}
  }
  if opts.SplitPanes && !record{
   // Same shared filter file format as Kubernetes tailg's --split-panes.
   // Both the text query and F1 matches-only mode remain synchronized.
   tmp:=filepath.Join(os.TempDir(),fmt.Sprintf("dynar-filter-%d-%d.txt",os.Getpid(),time.Now().UnixNano()))
   if err:=logui.InitializeSharedFilter(tmp,initialQuery(opts));err!=nil{return false,fmt.Errorf("initialize split-pane filter: %w",err)}
   opts.FilterFile=tmp
   names:=make([]string,0,len(apps))
   for _,app:=range apps{
    if len(selected)==0 || selected[app.Name]{names=append(names,app.Name)}
   }
   if err:=LaunchSplitPanes(names,opts);err!=nil{return false,err}
   fmt.Fprintf(stderr,"Opened %d DynaR workload panes in Windows Terminal.\n",len(names))
   return false,nil
  }
  list:=broker.Inventory(selected)
  if len(list)==0{fmt.Fprintln(stderr,"No streams available for selected workloads");continue}
  target:="all"
  if len(selected)>0{
   names:=make([]string,0,len(selected))
   for name:=range selected{names=append(names,name)}
   sort.Strings(names)
   target=strings.Join(names,",")
  }
  var recorder *Recorder
  if record {
   if recordFrom.IsZero(){recordFrom=time.Now()}
   var err error
   recorder,err=NewRecorder(".",opts.Namespace,opts.Source,target,recordFrom)
   if err!=nil{return false,fmt.Errorf("start live recording: %w",err)}
   fmt.Fprintln(stderr,"Recording live logs to",recorder.Path)
  }
  recordingPath:=""
  var capture func([]logui.Event)error
  if recorder!=nil{recordingPath=recorder.Path;capture=recorder.Write}
  err:=logui.Run(ctx,logui.Config{
   RecordingPath:recordingPath,Capture:capture,
   Title:"DynaR | "+opts.Source+" | "+opts.Namespace,
   Namespace:opts.Namespace,Provider:opts.Source,Target:target,
   Sources:list,RefreshInterval:opts.Refresh,BufferLines:opts.BufferLines,
   Color:true,ShowPod:true,UseConfiguredAI:true,
   InitialFilter:initialQuery(opts),FilterFile:opts.FilterFile,
   RegexFilter:func()string{if opts.Regex{return opts.Search};return ""}(),
   Stream:broker.Subscribe,
   Inventory:func(ctx context.Context)([]logui.Source,error){
    if err:=broker.Status();err!=nil{return nil,err}
    return broker.Inventory(selected),nil
   },
   Search:func(ctx context.Context,text string)([]logui.Event,error){
    return broker.Search(ctx,text,selected)
   },
   Trace:func(ctx context.Context,id string)([]logui.Event,error){
    // Distributed traces can cross workloads even when the picker currently
    // displays only one service. F6 investigates the complete namespace.
    return broker.Trace(ctx,id,nil)
   },
   ErrorLogs:broker.ErrorLogs,
   LogDetails:broker.LogDetails,
  })
  if recorder!=nil{
   closeErr:=recorder.Close()
   fmt.Fprintf(stderr,"DynaR recording saved: %s (%d events)\n",recorder.Path,recorder.Count)
   if err==nil{err=closeErr}
  }
  if err!=nil{return false,err}
  if ctx.Err()!=nil || opts.All || opts.Service!=""{return false,nil}
 }
}

// A portable fallback for DynaR-specific verified updates. Unlike tailg's
// updater this checks and installs assets from the independent dynar releases.
func promptDynaRUpdate(ctx context.Context)(restart,quit bool,err error){
 client:=update.NewSession(ctx,InstalledVersion())
 client.Cleanup()
 release,available:=client.Offer(ctx)
 if !available{return false,false,nil}
 // DynaR still owns the independently configured, SHA-256 verified updater.
 // tailg supplies only the modal view and keyboard interaction.
 return logui.PromptUpdate(ctx,logui.UpdateOffer{
  Application:"DynaR",
  Installed:client.Version,
  Latest:release.Version,
  Install:func(ctx context.Context)error{return client.Install(ctx,release)},
 })
}

func initialQuery(opts SharedOptions)string{
 initial:=""
 if !opts.Regex{initial=opts.Search}
 if opts.Level!=""{
  level:=strings.ToUpper(opts.Level)
  switch level{case "ERROR","ERR":level="ERR";case "WARNING","WARN","WRN":level="WRN";case "FATAL","FTL":level="FTL";case "INFO","INF":level="INF"}
  if initial!=""{initial+=" "}
  initial+="level:"+level
 }
 return initial
}
