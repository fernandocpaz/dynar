package app
import (
 "encoding/json"
 "bytes"
 "os"
 "path/filepath"
 "testing"
 "time"
 "github.com/fernandocpaz/tailg/pkg/logui"
)
func TestRecordingExcludesPreKeypressHistory(t *testing.T){
 dir:=t.TempDir()
 from:=time.Date(2026,11,23,21,0,0,0,time.UTC)
 rec,err:=NewRecorder(dir,"apollo","prod","encounter",from)
 if err!=nil{t.Fatal(err)}
 err=rec.Write([]logui.Event{
  {Timestamp:from.Add(-time.Second),Workload:"encounter",Message:"history"},
  {Timestamp:from,Workload:"encounter",Message:"first live log",TraceID:"abc"},
  {Timestamp:from.Add(time.Second),Workload:"encounter",Message:"second live log"},
 })
 if err!=nil{t.Fatal(err)}
 if err=rec.Close();err!=nil{t.Fatal(err)}
 raw,err:=os.ReadFile(rec.Path)
 if err!=nil{t.Fatal(err)}
 lines:=[]byte(nil)
 _=lines
 var decoded []map[string]any
 for _,line:=range bytes.Split(bytes.TrimSpace(raw),[]byte("\n")){
  var obj map[string]any
  if err=json.Unmarshal(line,&obj);err!=nil{t.Fatal(err)}
  decoded=append(decoded,obj)
 }
 if len(decoded)!=2||decoded[0]["message"]!="first live log"||decoded[1]["message"]!="second live log"{
  t.Fatalf("wrong F cutoff: %v",decoded)
 }
 if filepath.Dir(rec.Path)!=dir{t.Fatal("recording outside requested dir")}
}
