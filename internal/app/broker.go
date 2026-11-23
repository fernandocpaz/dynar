package app

import (
 "context"
 "errors"
 "fmt"
 "sort"
 "strings"
 "sync"
 "time"

 "github.com/fernandocpaz/dynar/internal/logs"
 "github.com/fernandocpaz/tailg/pkg/logui"
)

// Broker executes exactly one namespace-wide Grail query per polling cycle.
// Per-pod subscriptions consume that shared result, not their own DQL queries.
type Broker struct{
 mu sync.RWMutex
 collector Collector
 source,namespace,cluster string
 refresh,history time.Duration
 store *logs.Store
 subs map[string]map[chan logs.Event]struct{}
 lastEnd time.Time
 lastError error
 warnings []string
 overflow int64
}
func NewBroker(c Collector,source,namespace,cluster string,since,refresh time.Duration,buffer int)*Broker{
 if refresh<time.Second{refresh=10*time.Second}
 if since<=0{since=30*time.Minute}
 return &Broker{collector:c,source:source,namespace:namespace,cluster:cluster,
   refresh:refresh,history:since,store:logs.NewStore(buffer),subs:make(map[string]map[chan logs.Event]struct{})}
}
func(b *Broker) Bootstrap(ctx context.Context)error{
 end:=time.Now().UTC()
 events,warnings,err:=b.collector.Load(ctx,b.source,b.namespace,b.cluster,end.Add(-b.history),end)
 if err!=nil{return err}
 b.mu.Lock()
 defer b.mu.Unlock()
 b.store.Add(events)
 b.warnings=warnings
 b.lastEnd=end
 return nil
}
func (b *Broker) Run(ctx context.Context){
 ticker:=time.NewTicker(b.refresh)
 defer ticker.Stop()
 for {
  select {
  case <-ctx.Done():return
  case <-ticker.C:
   b.mu.RLock()
   previous:=b.lastEnd
   b.mu.RUnlock()
   if previous.IsZero(){continue}
   end:=time.Now().UTC()
   events,warnings,err:=b.collector.Load(ctx,b.source,b.namespace,b.cluster,previous.Add(-5*time.Minute),end)
   b.mu.Lock()
   if err!=nil{
    b.lastError=err
   }else{
    b.lastError=nil
    b.warnings=warnings
    // Never move the cursor past a failed query. Late ingestion is still
    // retrievable from the overlapping polling window.
    b.lastEnd=end
    newEvents:=b.store.Add(events)
    for _,ev:=range newEvents{
     for subscriber:=range b.subs[recordSource(ev).Key()]{
      select {case subscriber<-ev:default:b.overflow++}
     }
    }
   }
   b.mu.Unlock()
  }
 }
}
func recordSource(e logs.Event)logui.Source{
 pod:=e.Pod
 if pod==""{pod=e.Service+"-unknown"}
 container:=e.Container
 if container==""{container=e.Service}
 return logui.Source{Workload:e.Service,Pod:pod,Container:container}
}
func asLogUI(e logs.Event)logui.Event{
 s:=recordSource(e)
 return logui.Event{
  Workload:s.Workload,Pod:s.Pod,Container:s.Container,
  Message:e.Message,Level:e.Level,TraceID:e.TraceID,
  RequestID:e.RequestID,Timestamp:e.Timestamp,
 }
}
func (b *Broker) Inventory(selected map[string]bool)[]logui.Source{
 b.mu.RLock();defer b.mu.RUnlock()
 sources:=make(map[string]logui.Source)
 for _,e:=range b.store.Items{
  if len(selected)>0&&!selected[e.Service]{continue}
  s:=recordSource(e)
  sources[s.Key()]=s
 }
 out:=make([]logui.Source,0,len(sources))
 for _,s:=range sources{out=append(out,s)}
 sort.Slice(out,func(i,j int)bool{
  if out[i].Workload!=out[j].Workload{return out[i].Workload<out[j].Workload}
  return out[i].Key()<out[j].Key()
 })
 return out
}
func (b *Broker) Apps()[]logui.App{
 b.mu.RLock();defer b.mu.RUnlock()
 mapApps:=make(map[string]*logui.App)
 seenPods:=make(map[string]map[string]bool)
 for _,e:=range b.store.Items{
  name:=e.Service
  app:=mapApps[name]
  if app==nil{app=&logui.App{Name:name};mapApps[name]=app;seenPods[name]=make(map[string]bool)}
  if e.Timestamp.After(app.LastSeen){app.LastSeen=e.Timestamp}
  p:=recordSource(e).Pod
  if !seenPods[name][p]{app.Pods=append(app.Pods,p);seenPods[name][p]=true}
 }
 out:=make([]logui.App,0,len(mapApps))
 for _,app:=range mapApps{
  sort.Strings(app.Pods)
  out=append(out,*app)
 }
 sort.Slice(out,func(i,j int)bool{
  if !out[i].LastSeen.Equal(out[j].LastSeen){return out[i].LastSeen.After(out[j].LastSeen)}
  return out[i].Name<out[j].Name
 })
 return out
}
func (b *Broker) Subscribe(ctx context.Context,source logui.Source,out chan<- logui.Event)error{
 // Register before replaying buffered history: new records arriving during
 // the replay are queued instead of lost.
 sub:=make(chan logs.Event,8192)
 b.mu.Lock()
 history:=make([]logs.Event,0,256)
 for _,ev:=range b.store.Items{
  if recordSource(ev).Key()==source.Key(){history=append(history,ev)}
 }
 key:=source.Key()
 if b.subs[key]==nil{b.subs[key]=make(map[chan logs.Event]struct{})}
 b.subs[key][sub]=struct{}{}
 b.mu.Unlock()
 defer func(){b.mu.Lock();delete(b.subs[key],sub);if len(b.subs[key])==0{delete(b.subs,key)};b.mu.Unlock()}()
 send:=func(e logs.Event)error{
  select {case out<-asLogUI(e):return nil;case <-ctx.Done():return ctx.Err()}
 }
 for _,ev:=range history{if err:=send(ev);err!=nil{return err}}
 for {
  select{
  case <-ctx.Done():return nil
  case e:=<-sub:
   if recordSource(e).Key()!=source.Key(){continue}
   if err:=send(e);err!=nil{return err}
  }
 }
}
// LoadHistory deliberately searches outside the live ring buffer, bounded by
// the requested --since lookback. Incomplete results produce a clear failure.
func(b *Broker) LoadHistory(ctx context.Context)([]logs.Event,error){
 end:=time.Now().UTC()
 from:=end.Add(-b.history)
 records,warnings,err:=b.collector.Load(ctx,b.source,b.namespace,b.cluster,from,end)
 if err!=nil{return nil,err}
 if len(warnings)>0{return nil,fmt.Errorf("historical query incomplete: %s",strings.Join(warnings,"; "))}
 return records,nil
}
func (b *Broker) Search(ctx context.Context,term string,selected map[string]bool)([]logui.Event,error){
 records,err:=b.LoadHistory(ctx)
 if err!=nil{return nil,err}
 var candidates []logui.Event
 for _,e:=range records{
  if len(selected)>0&&!selected[e.Service]{continue}
  candidates=append(candidates,asLogUI(e))
 }
 // Full-history search and live filtering share tailg's parser and field
 // semantics (level:, service:, pod:, trace:, status:, duration:, etc.).
 return logui.MatchEvents(term,candidates)
}
func (b *Broker) Trace(ctx context.Context,id string,selected map[string]bool)([]logui.Event,error){
 records,err:=b.LoadHistory(ctx)
 if err!=nil{return nil,err}
 var out []logui.Event
 for _,e:=range records{
  if len(selected)>0&&!selected[e.Service]{continue}
  if strings.EqualFold(e.TraceID,id)||strings.Contains(strings.ToLower(e.Message),strings.ToLower(id)){out=append(out,asLogUI(e))}
 }
 return out,nil
}
func (b *Broker) ErrorLogs(ctx context.Context,pod string)([][]logui.Event,error){
 records,err:=b.LoadHistory(ctx)
 if err!=nil{return nil,err}
 var candidates []logui.Event
 for _,e:=range records{
  if recordSource(e).Pod==pod{candidates=append(candidates,asLogUI(e))}
 }
 return logui.ErrorBlocks(candidates),nil
}
func (b *Broker) Status()error{
 b.mu.RLock();defer b.mu.RUnlock()
 if b.lastError!=nil{return b.lastError}
 if b.overflow>0{return fmt.Errorf("%d live events missed by slow subscriber; reload or increase buffer",b.overflow)}
 if len(b.warnings)>0{return errors.New(strings.Join(b.warnings,"; "))}
 return nil
}
