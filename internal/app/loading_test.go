package app

import (
 "bytes"
 "errors"
 "strings"
 "testing"
)
func TestLoadingRedirectedIsPlainAndPreservesError(t *testing.T){
 var out bytes.Buffer
 want:=errors.New("query failed")
 calls:=0
 err:=withLoading(&out,"Loading logs...",func()error{calls++;return want})
 if err!=want||calls!=1||out.String()!="Loading logs...\n"{t.Fatalf("unexpected output %q, error %v, calls %d",out.String(),err,calls)}
}
func TestLoadingAnimationCompletesOnSuccessAndFailure(t *testing.T){
 for _,want:=range []error{nil,errors.New("failure")}{
  var out bytes.Buffer
  err:=runLoading(&out,"Loading logs...",true,func()error{return want})
  state:="[done]"
  if want!=nil{state="[failed]"}
  if err!=want||!strings.Contains(out.String(),state)||!strings.HasSuffix(out.String(),"\n"){t.Fatalf("unexpected final state %q, %v",out.String(),err)}
 }
}
