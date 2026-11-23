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
