package app

import "testing"

func TestSourceLabelShowsKubernetesCluster(t *testing.T){
 tests:=[]struct{
  name string
  opts SharedOptions
  want string
 }{
  {"prod cluster instead of Dynatrace source",SharedOptions{Source:"production-tenant",Cluster:"tkgs-prod",Namespace:"apollo"},"tkgs-prod"},
  {"QA cluster instead of Dynatrace source",SharedOptions{Source:"dynatrace-qa",Cluster:"tkgs-qa",Namespace:"apollo"},"tkgs-qa"},
  {"unscoped query",SharedOptions{Source:"prod",Namespace:"apollo"},"all clusters"},
 }
 for _,tc:=range tests{
  t.Run(tc.name,func(t *testing.T){
   got:=displaySourceCluster(tc.opts)
   if got!=tc.want{t.Fatalf("displaySourceCluster(%+v)=%q, want %q",tc.opts,got,tc.want)}
   // Display logic must never overwrite the configured Dynatrace source.
   if tc.opts.Source=="tkgs-prod"&&tc.name!="unscoped query"{t.Fatal("test setup conflates connection and cluster")}
  })
 }
}
