package app
import "testing"
func TestWindowExclusionScopeIsolation(t *testing.T){
 opts:=SharedOptions{Source:"tenant",Cluster:"example-production",Namespace:"demo-shop"}
 one:=windowExclusionFile("config.json",opts,map[string]bool{"checkout":true,"payments":true},nil)
 if one!=windowExclusionFile("config.json",opts,map[string]bool{"payments":true,"checkout":true},nil){t.Fatal("selection order changed profile")}
 if one==windowExclusionFile("config.json",opts,map[string]bool{"checkout":true},nil){t.Fatal("workload scope leaked")}
 if one==windowExclusionFile("config.json",opts,nil,map[string]bool{"checkout":true,"payments":true}){t.Fatal("pod scope leaked")}
 if one==windowExclusionFile("config.json",opts,nil,nil){t.Fatal("all scope leaked")}
 for _,other:=range []SharedOptions{
  {Source:"other",Cluster:opts.Cluster,Namespace:opts.Namespace},
  {Source:opts.Source,Cluster:"example-testing",Namespace:opts.Namespace},
  {Source:opts.Source,Cluster:opts.Cluster,Namespace:"inventory"},
 }{
  if one==windowExclusionFile("config.json",other,map[string]bool{"checkout":true,"payments":true},nil){t.Fatal("environment scope leaked")}
 }
}
