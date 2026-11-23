package app
import (
 "strings"
 "testing"
 "time"
)
func TestSplitPaneArgsUseExactWorkloadsAndPreserveSelection(t *testing.T){
 opts:=SharedOptions{Source:"qa",Namespace:"apollo",Cluster:"tkgs-qa",Since:time.Hour,Refresh:10*time.Second,Search:"SQL timeout",Level:"ERROR"}
 got:=SplitPaneArgs([]string{"encounter","encounter-dashboard","patient"},opts,"dynar.exe")
 if len(got)<40{t.Fatalf("expected three commands: %v",got)}
 joined:=strings.Join(got," | ")
 if strings.Count(joined,"--service-exact")!=3{t.Fatalf("missing exact-service child flag: %v",got)}
 if strings.Count(joined,"--no-update-check")!=3{t.Fatalf("duplicate update prompts: %v",got)}
 if strings.Count(joined,"split-pane")!=2{t.Fatalf("expected two splits: %v",got)}
 if !strings.Contains(joined,"--source | qa")||!strings.Contains(joined,"--cluster | tkgs-qa"){t.Fatalf("missing source scope: %v",got)}
 if !strings.Contains(joined,"SQL timeout"){t.Fatalf("search lost: %v",got)}
}
func TestSplitPaneArgsEmptySelection(t *testing.T){
 if got:=SplitPaneArgs(nil,SharedOptions{},"dynar.exe");got!=nil{t.Fatalf("expected no panes: %v",got)}
}
