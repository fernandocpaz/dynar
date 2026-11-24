package logs
import (
 "testing"
 "time"
)
func TestStoreChronologicalAndDedup(t *testing.T){
 now:=time.Now()
 a:=Event{Timestamp:now,Identity:"a",Service:"a"}
 b:=Event{Timestamp:now.Add(-time.Second),Identity:"b",Service:"b"}
 store:=NewStore(2)
 if n:=len(store.Add([]Event{a,b,a}));n!=2{t.Fatalf("got %d",n)}
 if store.Items[0].Identity!="b"{t.Fatalf("out of order: %v",store.Items)}
 c:=Event{Timestamp:now.Add(time.Second),Identity:"c"}
 store.Add([]Event{c})
 if len(store.Items)!=2||store.Items[0].Identity!="a"{t.Fatalf("eviction failed: %v",store.Items)}
}
func TestFromRecord(t *testing.T){
 e,err:=FromRecord(map[string]any{"timestamp":"2026-11-23T15:00:00.123Z","content":"hello","k8s.namespace.name":"apollo","k8s.deployment.name":"encounter","loglevel":"ERROR"},"qa")
 if err!=nil{t.Fatal(err)}
 if e.Namespace!="apollo"||e.Service!="encounter"||e.Level!="ERROR"||e.Identity==""{t.Fatalf("unexpected event: %+v",e)}
}

func TestReplayIdentitiesSurviveVisibleLogEviction(t *testing.T){
    now:=time.Now()
    store:=NewStore(2)
    store.Add([]Event{{Identity:"one",Timestamp:now},{Identity:"two",Timestamp:now.Add(time.Second)},{Identity:"three",Timestamp:now.Add(2*time.Second)}})
    if len(store.Items)!=2{t.Fatal("visible ring should stay bounded")}
    if got:=store.Add([]Event{{Identity:"one",Timestamp:now}});len(got)!=0{
        t.Fatalf("refetched log reappeared after visible eviction: %+v",got)
    }
    store.SeenMax=2
    store.Add([]Event{{Identity:"four",Timestamp:now.Add(3*time.Second)}})
    if len(store.seen)>2{t.Fatalf("identity memory budget exceeded: %d",len(store.seen))}
}

func TestFromRecordHTTPAttributesAreUnitSafe(t *testing.T){
  base:=map[string]any{
   "timestamp":"2026-11-23T15:00:00Z",
   "content":"completed request",
   "http.request.method":"GET",
   "url.path":"/patients/123",
   "http.response.status_code":float64(503),
   "duration_ms":float64(420),
  }
  event,err:=FromRecord(base,"prod")
  if err!=nil{t.Fatal(err)}
  if event.HTTPMethod!="GET"||event.HTTPPath!="/patients/123"||event.HTTPStatusCode!=503||
    !event.HasHTTPDuration||event.HTTPDuration!=420*time.Millisecond{
   t.Fatalf("enriched HTTP data lost: %+v",event)
  }
  base["duration_ms"]=nil
  base["duration"]=float64(1234) // numeric without unit: unknowable
  event,err=FromRecord(base,"prod")
  if err!=nil{t.Fatal(err)}
  if event.HasHTTPDuration{t.Fatalf("invented units for raw duration: %+v",event)}
  base["duration"]="250 ms"
  event,err=FromRecord(base,"prod")
  if err!=nil||!event.HasHTTPDuration||event.HTTPDuration!=250*time.Millisecond{
   t.Fatalf("duration string parsing: %+v, %v",event,err)
  }
}
func TestInvalidHTTPStatusNeverBecomesMetric(t *testing.T){
 event,err:=FromRecord(map[string]any{"timestamp":"2026-11-23T15:00:00Z","content":"ok","http.response.status_code":float64(999)},"qa")
 if err!=nil{t.Fatal(err)}
 if event.HTTPStatusCode!=0{t.Fatalf("accepted invalid HTTP code: %d",event.HTTPStatusCode)}
}
