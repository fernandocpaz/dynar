package logs

import (
    "strings"
    "testing"
)
func TestPrettySerilog(t *testing.T) {
    e:=Event{Message:"{\"@t\":\"2026-11-23T15:00:00Z\",\"@l\":\"Error\",\"@m\":\"SQL timeout\",\"@x\":\"System.Exception: Oops\\n at Handler\",\"SourceContext\":\"Encounter.Service\"}"}
    if GuessLevel(e.Message)!="ERROR"{t.Fatal("missing severity")}
    got:=PrettyMessage(e)
    if len(got)!=2||!strings.Contains(got[0],"[Encounter.Service] SQL timeout")||!strings.Contains(got[1]," at Handler"){t.Fatalf("bad formatting: %v",got)}
}
func TestPlainAndSafety(t *testing.T) {
    msg:="HTTP GET completed {\"TraceId\":\"abc\"}"
    if got:=PrettyMessage(Event{Message:msg});got[0]!="HTTP GET completed"{t.Fatalf("wrong condensed view: %q",got)}
    body:="Response Body: {\"status\":\"failed\"}"
    if got:=PrettyMessage(Event{Message:body});got[0]!=body{t.Fatalf("lost payload: %q",got)}
    unsafe:="\x1b[2J\x1b]0;danger\x07An ERROR occurred"
    if got:=SafeText(unsafe);strings.Contains(got,"\x1b")||!strings.Contains(got,"ERROR"){t.Fatalf("unsafe escape: %q",got)}
    if LevelCode(GuessLevel("2026-11-23 [ERR] failed"))!="ERR"{t.Fatal("severity")}
}
func TestPodGrouping(t *testing.T){
    if got:=WorkloadFromPod("vitas-emr-api-patient-7ab12ccc9-k9m2n");got!="vitas-emr-api-patient"{t.Fatal(got)}
    if got:=WorkloadFromPod("redis-0");got!="redis"{t.Fatal(got)}
}
