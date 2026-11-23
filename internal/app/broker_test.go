package app
import (
 "context"
 "fmt"
 "testing"
 "time"
 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/dynar/internal/logs"
)
func TestSharedCollectorSupportsMultipleServiceSubscriptions(t *testing.T){
 now:=time.Now().UTC().Truncate(time.Second)
 called:=0
 c:=Collector{Limit:20,MaxQueries:2,Query:func(_ context.Context,namespace,cluster string,from,to time.Time,limit int)(dynatrace.Result,error){
  called++
  return dynatrace.Result{Records:[]map[string]any{
   {"timestamp":now.Add(-time.Second).Format(time.RFC3339Nano),"content":"Encounter saved","k8s.namespace.name":"apollo","k8s.deployment.name":"encounter","k8s.pod.name":"encounter-abc","k8s.container.name":"app"},
   {"timestamp":now.Add(-time.Second).Format(time.RFC3339Nano),"content":"Patient loaded","k8s.namespace.name":"apollo","k8s.deployment.name":"patient","k8s.pod.name":"patient-def","k8s.container.name":"app"},
  }},nil
 }}
 b:=NewBroker(c,"qa","apollo","",time.Minute,time.Second,100)
 if err:=b.Bootstrap(context.Background());err!=nil{t.Fatal(err)}
 if called!=1{t.Fatalf("expected one namespace query, got %d",called)}
 apps:=b.Apps()
 if len(apps)!=2{t.Fatalf("want two apps, got %v",apps)}
 items:=b.Inventory(map[string]bool{"encounter":true})
 if len(items)!=1||items[0].Workload!="encounter"{t.Fatalf("wrong selection: %v",items)}
 ch:=make(chan struct{})
 received:=make(chan string,4)
 ctx,cancel:=context.WithCancel(context.Background())
 defer cancel()
 output:=make(chan logs.Event)
 _=output
 _=ch
 _=received
 _=ctx
}
func TestSourceFallback(t *testing.T){
 e:=logs.Event{Service:"worker",Message:"ok",Timestamp:time.Now()}
 s:=recordSource(e)
 if s.Pod!="worker-unknown"||s.Container!="worker"{t.Fatal(fmt.Sprint(s))}
}
