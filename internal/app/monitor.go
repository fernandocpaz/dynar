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

type logHealthSample struct{lines,warnings,errors int}
func(b *Broker) recentLogHealth(now time.Time)(map[string]logHealthSample,bool,string){
 b.mu.RLock()
 defer b.mu.RUnlock()
 counts:=make(map[string]logHealthSample)
 from:=now.Add(-time.Minute)
 // The visible buffer is bounded. When it evicts records from the last minute,
 // no rate or "healthy" verdict can safely be inferred from truncated counts.
 complete:=b.lastError==nil&&len(b.warnings)==0&&b.overflow==0&&
  !b.lastEnd.IsZero()&&now.Sub(b.lastEnd)<2*time.Minute
 if len(b.store.Items)>=b.store.Max&&len(b.store.Items)>0&&b.store.Items[0].Timestamp.After(from){
  complete=false
 }
 for _,event:=range b.store.Items {
  if event.Timestamp.Before(from)||event.Timestamp.After(now){continue}
  v:=counts[event.Service]
  v.lines++
  switch logs.LevelCode(event.Level){
  case "ERR","FTL":v.errors++
  case "WRN":v.warnings++
  }
  counts[event.Service]=v
 }
 warning:=""
 if !complete{
  warning="recent log sample incomplete (query limits, lag, or bounded buffer); health status unknown"
 }
 return counts,complete,warning
}

// Monitor obtains resource telemetry independently of the log token. Failure
// to read metrics preserves logging and clearly marks CPU/MEM as unavailable.
func(b *Broker) Monitor(ctx context.Context,client *dynatrace.Client)(logui.MonitorSnapshot,error){
 now:=time.Now().UTC()
 logCounts,logsComplete,logWarning:=b.recentLogHealth(now)
 samples:=make(map[string]logui.WorkloadSample)
 for _,app:=range b.Apps(){
  value:=logCounts[app.Name]
  samples[app.Name]=logui.WorkloadSample{
   Workload:app.Name,Lines:value.lines,Warnings:value.warnings,
   Errors:value.errors,LogsComplete:logsComplete,
  }
 }
 warnings:=make([]string,0,3)
 if logWarning!=""{warnings=append(warnings,logWarning)}
 if client==nil{
  warnings=append(warnings,"Dynatrace metrics not configured")
 }else{
  metrics,err:=client.QueryWorkloadMetrics(ctx,b.namespace,b.cluster)
  if err!=nil{
   warnings=append(warnings,"CPU/MEM unavailable: "+err.Error()+" (requires storage:metrics:read)")
  }else{
   warnings=append(warnings,metrics.Warnings...)
   for _,metric:=range metrics.Workloads{
    sample:=samples[metric.Name]
    sample.Workload=metric.Name
    sample.CPUPercent=metric.CPUPercent
    sample.CPUKnown=metric.CPUKnown
    sample.MemoryBytes=metric.MemoryBytes
    sample.MemoryKnown=metric.MemoryKnown
    sample.MemoryLimitBytes=metric.MemoryLimitBytes
    sample.MemoryLimitKnown=metric.MemoryLimitKnown
    samples[metric.Name]=sample
   }
  }
 }
 result:=logui.MonitorSnapshot{At:now,Warning:strings.Join(warnings," · ")}
 for _,app:=range b.Apps(){
  result.Workloads=append(result.Workloads,samples[app.Name])
 }
 return result,nil
}
func (b *Broker) monitorInfo()string{
 b.mu.RLock();defer b.mu.RUnlock()
 return fmt.Sprintf("namespace=%s cluster=%s",b.namespace,b.cluster)
}
