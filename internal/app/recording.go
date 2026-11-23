package app

import (
 "bufio"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "time"
 "unicode"
 "github.com/fernandocpaz/tailg/pkg/logui"
)

type Recorder struct{
 Path string
 Count int
 file *os.File
 buffer *bufio.Writer
 from time.Time
 source,namespace string
}
func filenamePart(input string)string{
 v:=strings.Map(func(r rune)rune{
  if unicode.IsLetter(r)||unicode.IsDigit(r)||r=='-'||r=='_'{return r}
  return '-'
 },input)
 v=strings.Trim(v,"-_")
 if v==""{v="all"}
 runes:=[]rune(v)
 if len(runes)>56{v=string(runes[:56])}
 return v
}
// NewRecorder creates a unique owner-only file and never overwrites an
// existing capture. The file is not created until the operator presses F.
func NewRecorder(dir,namespace,source,target string,from time.Time)(*Recorder,error){
 if from.IsZero(){return nil,errors.New("recording start timestamp is required")}
 abs,err:=filepath.Abs(dir)
 if err!=nil{return nil,err}
 stem:=fmt.Sprintf("dynar_%s_%s_%s_%s",filenamePart(source),filenamePart(namespace),filenamePart(target),from.Local().Format("20060102_150405"))
 for n:=0;n<100;n++{
  path:=filepath.Join(abs,stem+".ndjson")
  if n>0{path=filepath.Join(abs,fmt.Sprintf("%s_%02d.ndjson",stem,n))}
  file,err:=os.OpenFile(path,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
  if errors.Is(err,os.ErrExist){continue}
  if err!=nil{return nil,err}
  return &Recorder{Path:path,file:file,buffer:bufio.NewWriterSize(file,128*1024),from:from,source:source,namespace:namespace},nil
 }
 return nil,errors.New("all 100 recording filenames already exist")
}
func(r *Recorder)Write(events []logui.Event)error{
 if r==nil || r.file==nil{return errors.New("recording is closed")}
 encoder:=json.NewEncoder(r.buffer)
 for _,e:=range events{
  if e.Timestamp.IsZero() || e.Timestamp.Before(r.from){continue}
  record:=struct{
   Timestamp time.Time `json:"timestamp"`
   Source string `json:"source"`
   Namespace string `json:"namespace"`
   Workload string `json:"workload"`
   Pod string `json:"pod"`
   Container string `json:"container"`
   Level string `json:"level"`
   TraceID string `json:"trace_id,omitempty"`
   RequestID string `json:"request_id,omitempty"`
   Message string `json:"message"`
  }{e.Timestamp,r.source,r.namespace,e.Workload,e.Pod,e.Container,e.Level,e.TraceID,e.RequestID,e.Message}
  if err:=encoder.Encode(record);err!=nil{return err}
  r.Count++
 }
 return r.buffer.Flush()
}
func(r *Recorder)Close()error{
 if r==nil||r.file==nil{return nil}
 flushErr:=r.buffer.Flush()
 syncErr:=r.file.Sync()
 closeErr:=r.file.Close()
 r.file=nil
 if flushErr!=nil{return flushErr}
 if syncErr!=nil{return syncErr}
 return closeErr
}
