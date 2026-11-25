package app

import (
 "context"
 "fmt"
 "strings"
 "time"
 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/dynar/internal/logs"
 "github.com/fernandocpaz/tailg/pkg/logui"
)

type podLogSample struct{last time.Time;errors,warnings int}
func(b *Broker)podLogHealth(now time.Time)(map[string]podLogSample,bool){
 b.mu.RLock();defer b.mu.RUnlock()
 from:=now.Add(-time.Minute)
 complete:=b.lastError==nil&&len(b.warnings)==0&&b.overflow==0&&!b.lastEnd.IsZero()&&now.Sub(b.lastEnd)<2*time.Minute&&!b.cacheStart.After(from)
 if len(b.store.Items)>=b.store.Max&&len(b.store.Items)>0&&b.store.Items[0].Timestamp.After(from){complete=false}
 out:=map[string]podLogSample{}
 for _,e:=range b.store.Items{
  if e.Pod==""||e.Timestamp.After(now){continue}
  v:=out[e.Pod];if e.Timestamp.After(v.last){v.last=e.Timestamp}
  if !e.Timestamp.Before(from){switch logs.LevelCode(e.Level){case "ERR","FTL":v.errors++;case "WRN":v.warnings++}}
  out[e.Pod]=v
 }
 return out,complete
}

func(b *Broker)PodTelemetry(ctx context.Context,inventory,metrics *dynatrace.Client)([]logui.App,string,error){
 type inventoryResult struct{states map[string]dynatrace.PodState;err error}
 type metricResult struct{states map[string]dynatrace.WorkloadMetrics;err error}
 stateCh:=make(chan inventoryResult,1);metricCh:=make(chan metricResult,1)
 go func(){if inventory==nil{stateCh<-inventoryResult{err:fmt.Errorf("Smartscape not configured")};return};v,e:=inventory.QueryPodInventory(ctx,b.namespace,b.cluster);stateCh<-inventoryResult{v,e}}()
 go func(){if metrics==nil{metricCh<-metricResult{err:fmt.Errorf("metrics not configured")};return};v,e:=metrics.QueryPodMetrics(ctx,b.namespace,b.cluster);metricCh<-metricResult{v,e}}()
 states,resources:=<-stateCh,<-metricCh
 now:=time.Now().UTC()
 counts,complete:=b.podLogHealth(now)
 apps:=b.Apps()
 for i:=range apps{
  app:=&apps[i];known,ready:=0,0
  for _,name:=range app.Pods{
   p,ok:=states.states[name];info:=logui.PodInfo{Name:name,Ready:"?",Phase:"Unknown"}
   var details []string
   if ok{
    if p.Ready!=""{info.Ready=p.Ready}
    if p.Phase!=""{info.Phase=p.Phase}
    if p.Reason!=""&&p.Phase!="Terminating"{info.Phase=p.Reason}
    info.StartedAt=p.StartedAt
    if p.ConditionKnown{known++;if p.IsReady{ready++}}
    if p.RestartsKnown{details=append(details,fmt.Sprintf("Restarts %d",p.Restarts))}else{details=append(details,"Restarts ?")}
    if !p.LastRestart.IsZero(){details=append(details,"Last restart "+p.LastRestart.Local().Format("Jan 2 15:04"))}
    if !p.StartedAt.IsZero()&&!p.StartedAt.After(now){details=append(details,"Age "+now.Sub(p.StartedAt).Round(time.Minute).String())}
   }
   if m,ok:=resources.states[name];ok{
    if m.CPUKnown{details=append(details,fmt.Sprintf("CPU %.0f%% limit",m.CPUPercent))}else{details=append(details,"CPU ?")}
    if m.MemoryKnown&&m.MemoryLimitKnown&&m.MemoryLimitBytes>0{details=append(details,fmt.Sprintf("MEM %.0f%% limit",100*m.MemoryBytes/m.MemoryLimitBytes))}else if m.MemoryKnown{details=append(details,fmt.Sprintf("MEM %.0f MiB",m.MemoryBytes/(1024*1024)))}else{details=append(details,"MEM ?")}
   }
   sample:=counts[name]
   if complete&&!sample.last.IsZero(){details=append(details,fmt.Sprintf("ERR/min %d WRN/min %d",sample.errors,sample.warnings))}else{details=append(details,"ERR/min ? WRN/min ?")}
   if !sample.last.IsZero(){details=append(details,"Last log "+sample.last.Local().Format("Jan 2 15:04:05"))}
   if p.Node!=""{details=append(details,"Node "+p.Node)}
   if p.Images!=""{details=append(details,"Image "+p.Images)}
   info.Details=strings.Join(details," · ");app.PodInfo=append(app.PodInfo,info)
  }
  if known==len(app.Pods)&&known>0{app.Ready=fmt.Sprintf("%d/%d",ready,known)}else if known>0{app.Ready=fmt.Sprintf("%d/%d?",ready,len(app.Pods))}else{app.Ready="?"}
 }
 notes:=[]string{}
 if states.err!=nil{notes=append(notes,"Readiness unavailable (storage:smartscape:read)")}
 if resources.err!=nil{notes=append(notes,"CPU/MEM unavailable (storage:metrics:read)")}
 if len(notes)==0{notes=append(notes,"Pod telemetry updated "+now.Local().Format("15:04:05"))}
 return apps,strings.Join(notes," · "),nil
}
