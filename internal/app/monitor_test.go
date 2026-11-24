package app

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/dynar/internal/logs"
)
func TestMonitorRetainsErrorsWhenMetricsDenied(t *testing.T){
 now:=time.Now().UTC()
 b:=NewBroker(Collector{},"qa","apollo","",time.Minute,time.Second,100)
 b.mu.Lock()
 b.lastEnd=now
 b.store.Add([]logs.Event{{Identity:"1",Timestamp:now.Add(-2*time.Second),Service:"encounter",Level:"ERROR"},
  {Identity:"2",Timestamp:now.Add(-time.Second),Service:"encounter",Level:"WARN"}})
 b.mu.Unlock()
 mock:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(http.StatusForbidden)}))
 defer mock.Close()
 c,err:=dynatrace.NewMetricsClient(mock.URL,dynatrace.Credentials{Token:"test"})
 if err!=nil{t.Fatal(err)}
 c.HTTP=mock.Client()
 snapshot,err:=b.Monitor(context.Background(),c)
 if err!=nil{t.Fatal(err)}
 if len(snapshot.Workloads)!=1{t.Fatalf("missing workload: %+v",snapshot)}
 item:=snapshot.Workloads[0]
 if item.Errors!=1||item.Warnings!=1||!item.LogsComplete||item.CPUKnown||item.MemoryKnown{
  t.Fatalf("metrics failure impacted log health: %+v",item)
 }
 if !strings.Contains(snapshot.Warning,"CPU/MEM unavailable"){t.Fatalf("missing access notice: %s",snapshot.Warning)}
}
func TestMonitorBufferSaturationIsNeverHealthy(t *testing.T){
 now:=time.Now().UTC()
 b:=NewBroker(Collector{},"qa","apollo","",time.Minute,time.Second,2)
 b.mu.Lock()
 b.lastEnd=now
 b.store.Add([]logs.Event{{Identity:"1",Timestamp:now.Add(-3*time.Second),Service:"patient",Level:"INFO"},
  {Identity:"2",Timestamp:now.Add(-2*time.Second),Service:"patient",Level:"INFO"},
  {Identity:"3",Timestamp:now.Add(-time.Second),Service:"patient",Level:"INFO"}})
 b.mu.Unlock()
 count,complete,_:=b.recentLogHealth(now)
 if complete||count["patient"].lines!=2{t.Fatalf("truncated recent buffer must not imply health: %v %+v",complete,count)}
}
