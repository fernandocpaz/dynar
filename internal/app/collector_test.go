package app

import (
 "context"
 "fmt"
 "strings"
 "testing"
 "time"
 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/dynar/internal/logs"
)
func TestCollectorSplitsSaturatedIntervals(t *testing.T){
 start:=time.Date(2026,11,23,15,0,0,0,time.UTC)
 end:=start.Add(8*time.Second)
 calls:=0
 records:=make([]map[string]any,8)
 for i:=range records {records[i]=map[string]any{
  "timestamp":start.Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano),
  "content":fmt.Sprintf("log %d",i),
  "k8s.namespace.name":"apollo",
 }}
 c:=Collector{Limit:2,MaxQueries:32,Query:func(_ context.Context,ns,cl string,a,b time.Time,limit int)(dynatrace.Result,error){
  calls++
  var found []map[string]any
  for i,r:=range records {
   ts:=start.Add(time.Duration(i)*time.Second)
   if !ts.Before(a)&&ts.Before(b){found=append(found,r)}
  }
  truncated:=len(found)>limit
  if truncated{found=found[:limit]}
  return dynatrace.Result{Records:found,AtLimit:truncated||len(found)==limit},nil
 }}
 got,warnings,err:=c.Load(context.Background(),"qa","apollo","",start,end)
 if err!=nil{t.Fatal(err)}
 if len(got)!=8||len(warnings)!=0||calls<3 {t.Fatalf("got=%d warnings=%v calls=%d",len(got),warnings,calls)}
}
func TestCollectorExhaustionIsVisible(t *testing.T){
 t0:=time.Now().UTC().Truncate(time.Second)
 c:=Collector{Limit:1,MaxQueries:2,Query:func(_ context.Context,_,_ string,a,b time.Time,_ int)(dynatrace.Result,error){
  return dynatrace.Result{AtLimit:true,Records:[]map[string]any{{"timestamp":a.Format(time.RFC3339Nano),"content":"log"}}},nil
 }}
 _,warnings,err:=c.Load(context.Background(),"qa","apollo","",t0,t0.Add(time.Minute))
 if err!=nil{t.Fatal(err)}
 if len(warnings)==0{t.Fatal("expected incomplete warning")}
}
func TestMatcher(t *testing.T){
 match,err:=makeMatcher("timeout|failed",true,"ERROR","encounter")
 if err!=nil{t.Fatal(err)}
 if !match(logs.Event{Service:"Encounter",Level:"error",Message:"SQL TIMEOUT"}){t.Fatal("expected match")}
 if match(logs.Event{Service:"Patient",Level:"error",Message:"SQL TIMEOUT"}){t.Fatal("unexpected service match")}
 if _,err=makeMatcher("[",true,"","");err==nil{t.Fatal("expected regex error")}
 match,err=makeMatcher("trace-123",false,"","")
 if err!=nil{t.Fatal(err)}
 if !match(logs.Event{TraceID:"trace-123"}){t.Fatal("should search trace id")}
 if strings.Contains(format(logs.Event{Message:"a\nb\x1b"}),"\x1b"){t.Fatal("format allowed ANSI escape")}
}
