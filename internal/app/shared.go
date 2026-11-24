package app

import (
 "context"
 "os"
 "path/filepath"
 "fmt"
 "io"
 "sort"
 "runtime"
 "strings"
 "time"

 "github.com/fernandocpaz/dynar/internal/update"
 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/tailg/pkg/logui"
)

type SharedOptions struct {
 Source, Namespace, Cluster, Service, Pod, Search string
 Regex bool
 Since, Refresh time.Duration
 BufferLines int
 NoUpdateCheck bool
 All bool
 SplitPanes bool
 ServiceExact bool
 Level string
 FilterFile string
 MetricsClient *dynatrace.Client
 MonitorOnStart bool
}

// displaySourceCluster is UI-only: tailg labels the external-provider value
// "Source". In DynaR, that label identifies the selected Kubernetes cluster.
// The configured Dynatrace source profile still controls URL, credentials,
// Grail queries and recording metadata independently of this display text.
func displaySourceCluster(opts SharedOptions)string{
 if value:=strings.TrimSpace(opts.Cluster);value!=""{return value}
 return "all clusters"
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
  selectedPods:=map[string]bool{}
  picked:=false
  var record bool
  var recordFrom time.Time
  switch {
  case opts.All:selected=map[string]bool{}
  case opts.Pod!="":
   selectedPods[opts.Pod]=true
  case opts.Service!="":
   selected=make(map[string]bool)
   for _,app:=range apps{
    match:=strings.Contains(strings.ToLower(app.Name),strings.ToLower(opts.Service))
    if opts.ServiceExact{match=strings.EqualFold(app.Name,opts.Service)}
    if match{selected[app.Name]=true}
   }
   if len(selected)==0{return false,fmt.Errorf("no matching workload for --service %q",opts.Service)}
  default:
   picked=true
   choice,err:=logui.PickAppsWithMonitor(apps,displaySourceCluster(opts),opts.Namespace,func(ctx context.Context)(logui.MonitorSnapshot,error){
    return broker.Monitor(ctx,opts.MetricsClient)
   },opts.MonitorOnStart)
   if err!=nil{return false,err}
   if choice.Quit{return false,nil}
   if len(choice.Workloads)==0&&len(choice.Pods)==0{continue}
   for _,pod:=range choice.Pods{selectedPods[pod]=true}
   record=choice.Record
   recordFrom=choice.RecordFrom
   selected=make(map[string]bool,len(choice.Workloads))
   for _,app:=range choice.Workloads{selected[app]=true}
  }
  if shouldLaunchPanes(opts,picked,record,len(selected),len(selectedPods),runtime.GOOS){
   // Match tailg: multiple selected apps or exact pods open Windows Terminal
   // split panes automatically. File recording stays in one viewer.
   var names []string
   exactPods:=len(selectedPods)>0
   if exactPods{
    for pod:=range selectedPods{names=append(names,pod)}
   }else{
    for _,app:=range apps{
     if len(selected)==0||selected[app.Name]{names=append(names,app.Name)}
    }
   }
   if len(names)==0{return false,fmt.Errorf("no logs available for selected pods or workloads")}
   sort.Strings(names)
   tmp:=filepath.Join(os.TempDir(),fmt.Sprintf("dynar-filter-%d-%d.txt",os.Getpid(),time.Now().UnixNano()))
   if err:=logui.InitializeSharedFilter(tmp,initialQuery(opts));err!=nil{return false,fmt.Errorf("initialize split-pane filter: %w",err)}
   opts.FilterFile=tmp
   var err error
   if exactPods{err=LaunchPodSplitPanes(names,opts)}else{err=LaunchSplitPanes(names,opts)}
   if err!=nil{return false,err}
   fmt.Fprintf(stderr,"Opened %d DynaR live log panes in Windows Terminal.\n",len(names))
   return false,nil
  }
  list:=filterInventoryPods(broker.Inventory(selected),selectedPods)
  if len(list)==0{fmt.Fprintln(stderr,"No streams available for selected workloads");continue}
  target:="all"
  if len(selectedPods)>0{
   names:=make([]string,0,len(selectedPods))
   for pod:=range selectedPods{names=append(names,pod)}
   sort.Strings(names)
   target="pods/"+strings.Join(names,",")
  }else if len(selected)>0{
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
   Title:"DynaR | "+displaySourceCluster(opts)+" | "+opts.Namespace,
   Namespace:opts.Namespace,Provider:displaySourceCluster(opts),Target:target,
   Sources:list,RefreshInterval:opts.Refresh,BufferLines:opts.BufferLines,
   // Pod names remain in source metadata, not the rendered log prefix.
   Color:true,ShowPod:false,UseConfiguredAI:true,
   InitialFilter:initialQuery(opts),FilterFile:opts.FilterFile,
   RegexFilter:func()string{if opts.Regex{return opts.Search};return ""}(),
   Stream:broker.Subscribe,
   Inventory:func(ctx context.Context)([]logui.Source,error){
    if err:=broker.Status();err!=nil{return nil,err}
    return filterInventoryPods(broker.Inventory(selected),selectedPods),nil
   },
   Search:func(ctx context.Context,text string)([]logui.Event,error){
    found,err:=broker.Search(ctx,text,selected)
    if err!=nil{return nil,err}
    return filterSearchPods(found,selectedPods),nil
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
  if ctx.Err()!=nil || opts.All || opts.Service!="" || opts.Pod!=""{return false,nil}
 }
}


// shouldLaunchPanes reproduces tailg picker behavior: on Windows, selecting
// multiple applications or exact pods opens individual live log panes.
// Explicit --split-panes remains available for one or many workloads.
func shouldLaunchPanes(opts SharedOptions,picked,record bool,workloads,pods int,goos string)bool{
 if record{return false}
 if opts.SplitPanes{return true}
 return picked&&goos=="windows"&&(workloads>1||pods>1)
}
func filterInventoryPods(all []logui.Source,selected map[string]bool)[]logui.Source{
 if len(selected)==0{return all}
 out:=make([]logui.Source,0,len(all))
 for _,source:=range all{if selected[source.Pod]{out=append(out,source)}}
 return out
}
func filterSearchPods(all []logui.Event,selected map[string]bool)[]logui.Event{
 if len(selected)==0{return all}
 out:=make([]logui.Event,0,len(all))
 for _,event:=range all{if selected[event.Pod]{out=append(out,event)}}
 return out
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
