package app

import (
 "context"
 "fmt"
 "sort"
 "strings"
 "time"

 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/dynar/internal/logs"
 "github.com/fernandocpaz/tailg/pkg/logui"
)

func sortedTargets(values map[string]bool)[]string{
 out:=make([]string,0,len(values))
 for k,yes:=range values{if yes{out=append(out,k)}}
 sort.Strings(out)
 return out
}
func selectedTimeEvent(e logs.Event,workloads,pods map[string]bool,from,to time.Time)bool{
 if e.Timestamp.Before(from)||!e.Timestamp.Before(to){return false}
 if len(workloads)>0&&!workloads[e.Service]{return false}
 if len(pods)>0&&!pods[recordSource(e).Pod]{return false}
 return true
}

// LoadTimeRange uses the broker's complete retained interval when possible.
// For missing history it queries only the selected targets and exact interval.
// Remote results are bounded by the existing collector's record/query budget,
// then by the configured viewer buffer. No log data is written to disk.
func(b *Broker)LoadTimeRange(ctx context.Context,client *dynatrace.Client,workloads,pods map[string]bool,from,to time.Time)(logui.TimeResult,error){
 if !to.After(from){return logui.TimeResult{},fmt.Errorf("invalid time window")}
 b.mu.RLock()
 capacity:=b.store.Max
 cached:=b.cacheComplete&&b.lastError==nil&&!from.Before(b.cacheStart)&&!to.After(b.lastEnd)
 if cached{
  events:=make([]logui.Event,0)
  for _,e:=range b.store.Items{
   if selectedTimeEvent(e,workloads,pods,from,to){events=append(events,asLogUI(e))}
  }
  b.mu.RUnlock()
  return logui.TimeResult{Events:events},nil
 }
 b.mu.RUnlock()
 collector:=b.collector
 if client!=nil&&(len(workloads)>0||len(pods)>0){
  ws,ps:=sortedTargets(workloads),sortedTargets(pods)
  collector.Query=func(ctx context.Context,ns,cluster string,start,end time.Time,limit int)(dynatrace.Result,error){
   return client.QueryTargets(ctx,ns,cluster,start,end,limit,ws,ps)
  }
 }
 records,warnings,err:=collector.Load(ctx,b.source,b.namespace,b.cluster,from,to)
 if err!=nil{return logui.TimeResult{},err}
 filtered:=make([]logs.Event,0,len(records))
 for _,record:=range records{if selectedTimeEvent(record,workloads,pods,from,to){filtered=append(filtered,record)}}
 store:=logs.NewStore(capacity)
 unique:=store.Add(filtered)
 if len(unique)>capacity{warnings=append(warnings,fmt.Sprintf("Display limited to newest %d records; narrow the time range",capacity))}
 events:=make([]logui.Event,0,len(store.Items))
 for _,record:=range store.Items{events=append(events,asLogUI(record))}
 return logui.TimeResult{Events:events,Warning:strings.Join(warnings,"; ")},nil
}
