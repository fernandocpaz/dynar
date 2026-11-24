package app

import(
 "context"
 "strings"
 "testing"
 "time"
 "github.com/fernandocpaz/dynar/internal/dynatrace"
)
func TestScopeDiscoveryCacheAndRanking(t *testing.T){
 c:=newScopeCatalog(nil,"")
 calls:=0
 c.query=func(_ context.Context,q string,from,to time.Time,limit int)(dynatrace.Result,error){
  calls++
  if !strings.Contains(q,`k8s.cluster.name == "example-production"`)||!strings.Contains(q,"dedup k8s.namespace.name")||to.Sub(from)!=24*time.Hour||limit!=scopeLimit{t.Fatalf("bad discovery query: %s",q)}
  return dynatrace.Result{Records:[]map[string]any{{"k8s.namespace.name":"inventory"},{"k8s.namespace.name":"demo-shop"},{"k8s.namespace.name":"payments"}}},nil
 }
 c.remember("example-production","payments")
 names,_,err:=c.list(context.Background(),"namespace","example-production","demo-shop")
 if err!=nil||len(names)!=3||names[0]!="demo-shop"||names[1]!="payments"{t.Fatalf("unexpected names %v: %v",names,err)}
 c.list(context.Background(),"namespace","example-production","demo-shop")
 if calls!=1{t.Fatalf("cache missed: %d",calls)}
}
func TestScopeChangesOnlyQueryScope(t *testing.T){
 before:=SharedOptions{Source:"tenant",Namespace:"demo-shop",Cluster:"example-production",Search:"timeout",Since:time.Hour,FilterFile:"old"}
 next,err:=changeScope(before,"cluster","")
 if err!=nil||next.Cluster!=""||next.Namespace!=before.Namespace||next.Source!=before.Source||next.Search!=before.Search||next.Since!=before.Since||next.FilterFile!=""{t.Fatalf("incorrect scope: %+v: %v",next,err)}
 next,err=changeScope(next,"namespace","payments")
 if err!=nil||next.Namespace!="payments"||next.Cluster!=""{t.Fatal("namespace switch failed")}
 if _,err=changeScope(next,"namespace","");err==nil{t.Fatal("accepted empty namespace")}
}
func TestClusterDiscoveryIsUnscopedAndEscaped(t *testing.T){
 q,field,err:=scopeDQL("cluster","ignored")
 if err!=nil||field!="k8s.cluster.name"||strings.Contains(q,"ignored"){t.Fatal(q)}
 q,_,err=scopeDQL("namespace",`a"b`)
 if err!=nil||!strings.Contains(q,`"a\"b"`){t.Fatal(q)}
}
