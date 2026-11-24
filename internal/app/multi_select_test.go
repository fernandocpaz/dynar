package app

import (
 "strings"
 "testing"

 "github.com/fernandocpaz/tailg/pkg/logui"
)

func TestAutoSplitMatchesTailgPickerOnWindows(t *testing.T){
 opts:=SharedOptions{}
 cases:=[]struct{
  title,os string
  picked,record bool
  workloads,pods int
  want bool
 }{
  {"two apps on Windows","windows",true,false,2,0,true},
  {"three exact pods on Windows","windows",true,false,0,3,true},
  {"one workload on Windows","windows",true,false,1,0,false},
  {"one exact pod on Windows","windows",true,false,0,1,false},
  {"two apps on WSL","linux",true,false,2,0,false},
  {"two pods on Mac","darwin",true,false,0,2,false},
  {"recording multiple","windows",true,true,3,0,false},
  {"script/non-picker selection","windows",false,false,3,0,false},
 }
 for _,tc:=range cases{t.Run(tc.title,func(t *testing.T){
  got:=shouldLaunchPanes(opts,tc.picked,tc.record,tc.workloads,tc.pods,tc.os)
  if got!=tc.want{t.Fatalf("auto split=%t, want %t",got,tc.want)}
 })}
 if !shouldLaunchPanes(SharedOptions{SplitPanes:true},false,false,1,0,"windows"){
  t.Fatal("explicit --split-panes should continue launching a pane")
 }
 if shouldLaunchPanes(SharedOptions{SplitPanes:true},true,true,2,0,"windows"){
  t.Fatal("F file-recording must never open extra processes")
 }
}
func TestExactPodSelectionExcludesOtherReplicas(t *testing.T){
 all:=[]logui.Source{
  {Workload:"encounter",Pod:"encounter-a",Container:"app"},
  {Workload:"encounter",Pod:"encounter-b",Container:"app"},
  {Workload:"patient",Pod:"patient-1",Container:"app"},
 }
 filtered:=filterInventoryPods(all,map[string]bool{"encounter-b":true})
 if len(filtered)!=1||filtered[0].Pod!="encounter-b"{t.Fatalf("wrong pod sources: %+v",filtered)}
 events:=[]logui.Event{
  {Pod:"encounter-a",Message:"other replica"},
  {Pod:"encounter-b",Message:"selected"},
  {Pod:"patient-1",Message:"other app"},
 }
 found:=filterSearchPods(events,map[string]bool{"encounter-b":true})
 if len(found)!=1||found[0].Message!="selected"{t.Fatalf("wrong scoped search: %+v",found)}
 if len(filterInventoryPods(all,nil))!=len(all){t.Fatal("all view should not exclude pods")}
}
func TestSplitPodPaneArgsKeepIndependentExactPodSources(t *testing.T){
 opts:=SharedOptions{
  Source:"prod",Namespace:"apollo",Cluster:"tkgs-prod",
  Search:"SQL timeout",FilterFile:"C:/Temp/shared-filter.txt",
 }
 names:=[]string{"encounter-a","encounter-b","patient-1"}
 args:=SplitPodPaneArgs(names,opts,"dynar.exe")
 flat:=strings.Join(args,"|")
 if strings.Count(flat,"--pod-exact")!=len(names){t.Fatalf("one pod flag per pane expected: %v",args)}
 if strings.Contains(flat,"--service")||strings.Contains(flat,"--service-exact"){
  t.Fatalf("pod pane accidentally selects entire workload: %v",args)
 }
 if strings.Count(flat,"--cluster")!=len(names)||strings.Count(flat,"tkgs-prod")!=len(names){
  t.Fatalf("cluster filter not inherited: %v",args)
 }
 if strings.Count(flat,"--filter-file")!=len(names){t.Fatalf("F1 sync lost: %v",args)}
 if strings.Count(flat,"--no-update-check")!=len(names){t.Fatalf("child dialogs not suppressed: %v",args)}
 if strings.Count(flat,"split-pane")!=len(names)-1{t.Fatalf("expected separate panes: %v",args)}
 for _,name:=range names{
  if !strings.Contains(flat,"--pod-exact|"+name){t.Fatalf("missing exact pod %s: %v",name,args)}
 }
}
