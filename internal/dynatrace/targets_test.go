package dynatrace

import (
 "strings"
 "testing"
)
func TestScopedLogsDQL(t *testing.T){
 dql:=scopedLogsDQL("apollo","tkgs-prod",100,[]string{"encounter"},nil)
 for _,fragment:=range []string{"k8s.namespace.name == \"apollo\"","k8s.cluster.name == \"tkgs-prod\"","k8s.workload.name == \"encounter\"","startsWith(k8s.pod.name, \"encounter-\")","span_id","limit 100"}{
  if !strings.Contains(dql,fragment){t.Fatalf("missing %q in %s",fragment,dql)}
 }
 pod:=scopedLogsDQL("apollo","",100,[]string{"ignored"},[]string{"encounter-1"})
 if !strings.Contains(pod,"k8s.pod.name == \"encounter-1\"")||strings.Contains(pod,"\"ignored\""){t.Fatal("exact pod selection not preserved")}
 if targetPredicate(nil,[]string{"workload-unknown"})!=""{t.Fatal("synthetic pod names must fall back to namespace query")}
}
