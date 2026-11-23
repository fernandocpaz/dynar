package app

import (
 "context"
 "bufio"
 "os"
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
}

func sharedViewer(ctx context.Context,c Collector,opts SharedOptions,stdout,stderr io.Writer)(bool,error){
 if !opts.NoUpdateCheck && !isUpdateCheckDisabled() && StableBuild(){
  restart,err:=promptDynaRUpdate(ctx,stderr)
  if err!=nil{fmt.Fprintln(stderr,"DynaR update check:",err)}
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
    if strings.Contains(strings.ToLower(app.Name),strings.ToLower(opts.Service)){
     selected[app.Name]=true
    }
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
   InitialFilter:func()string{if opts.Regex{return ""};return opts.Search}(),
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
    return broker.Trace(ctx,id,selected)
   },
   ErrorLogs:broker.ErrorLogs,
   LogDetails:func(ctx context.Context,e logui.Event)([]logui.Event,error){
    return []logui.Event{e},nil
   },
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
func promptDynaRUpdate(ctx context.Context,stderr io.Writer)(bool,error){
 client:=update.NewSession(ctx,InstalledVersion())
 release,available:=client.Offer(ctx)
 if !available{return false,nil}
 fmt.Fprintf(stderr,"\nDynaR update available: %s → %s\n",client.Version,release.Version)
 fmt.Fprint(stderr,"[Enter] Later    [u + Enter] Update & restart: ")
 input,err:=bufio.NewReader(os.Stdin).ReadString('\n')
 if err!=nil && len(input)==0{return false,err}
 if !strings.EqualFold(strings.TrimSpace(input),"u"){return false,nil}
 fmt.Fprintln(stderr,"Downloading and verifying update...")
 if err:=client.Install(ctx,release);err!=nil{return false,err}
 return true,nil
}
