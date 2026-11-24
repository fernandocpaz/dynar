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

func TestHistoryQueriesUseTailgFieldFiltersAndErrorBlocks(t *testing.T){
    now:=time.Now().UTC().Truncate(time.Second)
    stamps:=func(n int)string{return now.Add(-time.Duration(4-n)*time.Second).Format(time.RFC3339Nano)}
    records:=[]map[string]any{
        {"timestamp":stamps(0),"k8s.deployment.name":"encounter","k8s.pod.name":"encounter-abcd","k8s.container.name":"app","content":"SQL failed","loglevel":"ERROR"},
        {"timestamp":stamps(1),"k8s.deployment.name":"encounter","k8s.pod.name":"encounter-abcd","k8s.container.name":"app","content":"   at Handler.Save()","loglevel":"INFO"},
        {"timestamp":stamps(2),"k8s.deployment.name":"patient","k8s.pod.name":"patient-efgh","k8s.container.name":"app","content":"Patient loaded","loglevel":"INFO"},
    }
    c:=Collector{Limit:50,MaxQueries:2,Query:func(_ context.Context,_,_ string,_,_ time.Time,_ int)(dynatrace.Result,error){
        return dynatrace.Result{Records:records},nil
    }}
    b:=NewBroker(c,"prod","apollo","",time.Minute,time.Second,100)
    found,err:=b.Search(context.Background(),"level:ERR service:encounter",nil)
    if err!=nil||len(found)!=1{t.Fatalf("structured search mismatch: %d: %v",len(found),err)}
    blocks,err:=b.ErrorLogs(context.Background(),"encounter-abcd")
    if err!=nil||len(blocks)!=1||len(blocks[0])!=2{
        t.Fatalf("lost split exception frames: %#v, %v",blocks,err)
    }
}

func TestLogDetailsRestoresRetainedStackFrames(t *testing.T){
 now:=time.Now().UTC().Truncate(time.Second)
 records:=[]map[string]any{
  {"timestamp":now.Add(-4*time.Second).Format(time.RFC3339Nano),"content":"[ERR] failure","k8s.pod.name":"encounter-pod","k8s.container.name":"app","k8s.deployment.name":"encounter","loglevel":"ERROR"},
  {"timestamp":now.Add(-3*time.Second).Format(time.RFC3339Nano),"content":"    at Work.Run()","k8s.pod.name":"encounter-pod","k8s.container.name":"app","k8s.deployment.name":"encounter"},
  {"timestamp":now.Add(-2*time.Second).Format(time.RFC3339Nano),"content":"[INF] success","k8s.pod.name":"encounter-pod","k8s.container.name":"app"},
 }
 c:=Collector{Limit:100,MaxQueries:5,Query:func(_ context.Context,_,_ string,_,_ time.Time,_ int)(dynatrace.Result,error){return dynatrace.Result{Records:records},nil}}
 broker:=NewBroker(c,"prod","apollo","",time.Minute,time.Second,100)
 selected:=asLogUI(logs.Event{Timestamp:now.Add(-4*time.Second),Message:"[ERR] failure",Pod:"encounter-pod",Container:"app",Service:"encounter"})
 got,err:=broker.LogDetails(context.Background(),selected)
 if err!=nil || len(got)!=2{t.Fatalf("complete exception block missing: %v %v",got,err)}
}

func TestGrailHTTPAttributesForwardedToSharedF9Engine(t *testing.T){
 at:=time.Now().UTC()
 ev,err:=logs.FromRecord(map[string]any{
  "timestamp":at.Format(time.RFC3339Nano),
  "content":"request completed","k8s.deployment.name":"encounter",
  "http.request.method":"POST","url.path":"/encounters/7",
  "http.response.status_code":float64(500),"duration_ms":float64(400),
 },"prod")
 if err!=nil{t.Fatal(err)}
 ui:=asLogUI(ev)
 if ui.HTTPMethod!="POST"||ui.HTTPPath!="/encounters/7"||ui.HTTPStatusCode!=500||
  !ui.HasHTTPDuration||ui.HTTPDuration!=400*time.Millisecond{
  t.Fatalf("F9 metadata lost on provider bridge: %+v",ui)
 }
}
