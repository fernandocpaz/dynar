package dynatrace
import("testing";"strings")
func TestPodInventoryReadiness(t *testing.T){
 rows:=[]map[string]any{
  {"pod":"p","phase":"Running","desired":float64(1),"status":map[string]any{"conditions":[]any{map[string]any{"type":"Ready","status":"False"}},"containerStatuses":[]any{map[string]any{"ready":false,"restartCount":float64(3),"state":map[string]any{"waiting":map[string]any{"reason":"CrashLoopBackOff"}}}}}},
  {"pod":"q","phase":"Running"},
 }
 m:=ParsePodInventory(rows)
 if m["p"].Ready!="0/1 NotReady"||!m["p"].ConditionKnown||m["p"].IsReady||m["p"].Restarts!=3||m["p"].Reason!="CrashLoopBackOff"{t.Fatalf("incorrect pod: %+v",m["p"])}
 if m["q"].ConditionKnown||m["q"].RestartsKnown{t.Fatal("missing data was treated as known")}
 rows=append(rows,rows[0]);if _,ok:=ParsePodInventory(rows)["p"];ok{t.Fatal("ambiguous pod")}
}
func TestPodInventoryScope(t *testing.T){
 q:=PodInventoryDQL("ns","cluster",10)
 for _,s:=range []string{"smartscapeNodes K8S_POD","k8s.namespace.name == \"ns\"","k8s.cluster.name == \"cluster\"","limit 10"}{if !strings.Contains(q,s){t.Fatal(q)}}
}
