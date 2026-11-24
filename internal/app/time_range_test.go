package app

import (
 "context"
 "testing"
 "time"

 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/dynar/internal/logs"
)

func TestTimeRangeUsesCompleteCacheAndSelectsExactSource(t *testing.T){
 now:=time.Now().UTC()
 queries:=0
 b:=NewBroker(Collector{Limit:10,MaxQueries:4,Query:func(context.Context,string,string,time.Time,time.Time,int)(dynatrace.Result,error){
  queries++;return dynatrace.Result{},nil
 }},"prod","apollo","tkgs-prod",time.Hour,time.Second,100)
 b.cacheStart=now.Add(-time.Hour);b.lastEnd=now;b.cacheComplete=true
 b.store.Add([]logs.Event{
  {Identity:"one",Timestamp:now.Add(-15*time.Minute),Service:"encounter",Pod:"encounter-1"},
  {Identity:"two",Timestamp:now.Add(-10*time.Minute),Service:"patient",Pod:"patient-1"},
  {Identity:"old",Timestamp:now.Add(-50*time.Minute),Service:"encounter",Pod:"encounter-1"},
 })
 result,err:=b.LoadTimeRange(context.Background(),nil,map[string]bool{"encounter":true},nil,now.Add(-30*time.Minute),now)
 if err!=nil||len(result.Events)!=1||result.Events[0].Pod!="encounter-1"||queries!=0{t.Fatalf("cache/select failed: %+v %v, queries=%d",result,err,queries)}
}
func TestTimeRangeFetchesOutsideBufferWithExactBounds(t *testing.T){
 now:=time.Now().UTC()
 from,to:=now.Add(-3*time.Hour),now.Add(-2*time.Hour)
 queries:=0
 b:=NewBroker(Collector{Limit:10,MaxQueries:4,Query:func(_ context.Context,ns,cluster string,start,end time.Time,limit int)(dynatrace.Result,error){
  queries++
  if ns!="apollo"||cluster!="tkgs-prod"||!start.Equal(from)||!end.Equal(to){t.Fatal("historical query bounds/scope lost")}
  return dynatrace.Result{Records:[]map[string]any{
   {"timestamp":from.Add(time.Minute).Format(time.RFC3339Nano),"k8s.workload.name":"encounter","k8s.pod.name":"encounter-1","content":"found"},
   {"timestamp":from.Add(time.Minute).Format(time.RFC3339Nano),"k8s.workload.name":"patient","k8s.pod.name":"patient-1","content":"other"},
  }},nil
 }},"prod","apollo","tkgs-prod",time.Hour,time.Second,100)
 result,err:=b.LoadTimeRange(context.Background(),nil,nil,map[string]bool{"encounter-1":true},from,to)
 if err!=nil||len(result.Events)!=1||queries!=1{t.Fatalf("historical query failed: %+v %v, queries=%d",result,err,queries)}
}
