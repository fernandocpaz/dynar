package logs

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "sort"
 "strconv"
 "math"
 "strings"
 "time"
)
type Event struct {
 Timestamp time.Time `json:"timestamp"`
 Source string `json:"source"`
 Namespace string `json:"namespace,omitempty"`
 Service string `json:"service"`
 Pod string `json:"pod,omitempty"`
 Container string `json:"container,omitempty"`
 Level string `json:"level"`
 Message string `json:"message"`
 TraceID string `json:"trace_id,omitempty"`
 SpanID string `json:"span_id,omitempty"`
 RequestID string `json:"request_id,omitempty"`
 // These optional attributes come from enriched Grail log records; absent
 // attributes must never be interpreted as a successful HTTP request.
 HTTPMethod string `json:"http_method,omitempty"`
 HTTPPath string `json:"http_path,omitempty"`
 HTTPStatusCode int `json:"http_status_code,omitempty"`
 HTTPDuration time.Duration `json:"-"`
 HasHTTPDuration bool `json:"-"`
 Identity string `json:"-"`
}
func stringField(rec map[string]any, keys ...string) string {
 for _,k:=range keys {
  if v,ok:=rec[k];ok && v!=nil {
   switch n:=v.(type) {
    case string:if n!=""{return n}
    case float64:return fmt.Sprint(n)
    case json.Number:return string(n)
   }
  }
 }
 return ""
}
func FromRecord(rec map[string]any, source string)(Event,error){
 raw:=stringField(rec,"timestamp")
 at,err:=time.Parse(time.RFC3339Nano,raw)
 if err!=nil{return Event{},fmt.Errorf("unexpected Dynatrace timestamp %q: %w",raw,err)}
 e:=Event{
  Timestamp:at, Source:source,
  Namespace:stringField(rec,"k8s.namespace.name"),
  Service:stringField(rec,"k8s.workload.name","k8s.deployment.name","k8s.statefulset.name","k8s.daemonset.name","k8s.job.name","k8s.cronjob.name","k8s.container.name","dt.process.name"),
  Pod:stringField(rec,"k8s.pod.name"),
  Container:stringField(rec,"k8s.container.name"),
  Level:strings.ToUpper(stringField(rec,"loglevel","severity","level")),
  Message:stringField(rec,"content","message","log"),
  TraceID:stringField(rec,"trace_id","trace.id"),
  SpanID:stringField(rec,"span_id","span.id"),
  RequestID:stringField(rec,"request_id","request.id"),
  HTTPMethod:stringField(rec,"http.request.method","http.method","request_method"),
  HTTPPath:stringField(rec,"url.path","http.route","http.target","requestpath"),
  HTTPStatusCode:parseHTTPStatus(rec),
 }
 e.HTTPDuration,e.HasHTTPDuration=parseHTTPDuration(rec)
 if e.Service==""{e.Service=WorkloadFromPod(e.Pod)}
 if e.Service==""{e.Service="unknown"}
 if e.Level==""{e.Level=GuessLevel(e.Message)}
 if e.Message==""{e.Message="(empty message)"}
 native:=stringField(rec,"id","event.id")
 if native!=""{e.Identity=source+"/"+native}else{
  hash:=sha256.Sum256([]byte(e.Timestamp.UTC().Format(time.RFC3339Nano)+"\x00"+source+"\x00"+e.Pod+"\x00"+e.Container+"\x00"+e.Message+"\x00"+e.TraceID+"\x00"+e.SpanID))
  e.Identity=hex.EncodeToString(hash[:])
 }
 return e,nil
}

// parseHTTPStatus accepts only explicit HTTP status attributes and valid HTTP
// response codes. A generic log "status" field is not necessarily HTTP.
func parseHTTPStatus(record map[string]any)int{
 value:=stringField(record,"http.response.status_code","http.status_code")
 n,err:=strconv.Atoi(value)
 if err!=nil||n<100||n>599{return 0}
 return n
}
func parseHTTPDuration(record map[string]any)(time.Duration,bool){
 // Untyped numeric "duration" values are ambiguous; do not infer units.
 // Prefer strings with units, or fields explicitly named in milliseconds.
 keys:=[]string{"duration","http.server.request.duration","http.duration_ms","duration_ms","elapsed_ms"}
 for _,key:=range keys{
  raw,ok:=record[key]
  if !ok||raw==nil{continue}
  if value,ok:=raw.(string);ok{
   text:=strings.ReplaceAll(strings.TrimSpace(value)," ","")
   if d,err:=time.ParseDuration(text);err==nil&&d>=0{return d,true}
  }
  // Numbers in fields ending _ms have explicit millisecond units.
  if !strings.HasSuffix(key,"_ms"){continue}
  var n float64
  switch v:=raw.(type){
  case float64:n=v
  case float32:n=float64(v)
  case int:n=float64(v)
  case int64:n=float64(v)
  case json.Number:
   number,err:=v.Float64()
   if err!=nil{continue}
   n=number
  case string:
   number,err:=strconv.ParseFloat(strings.TrimSpace(v),64)
   if err!=nil{continue}
   n=number
  default:continue
  }
  if math.IsNaN(n)||math.IsInf(n,0)||n<0||n>float64(1<<63-1)/float64(time.Millisecond){continue}
  return time.Duration(n*float64(time.Millisecond)),true
 }
 return 0,false
}

type Store struct {
 Max int
 SeenMax int // bounded replay identity budget, independent from the visible ring
 Items []Event
 seen map[string]struct{}
 seenOrder []string
 seenHead int
}
func NewStore(max int)*Store{
 if max<1{max=10000}
 seenMax:=max*8
 if seenMax<20000{seenMax=20000}
 if seenMax>1000000{seenMax=1000000}
 return &Store{Max:max,SeenMax:seenMax,seen:map[string]struct{}{}}
}
func(s *Store)Add(items []Event)[]Event{
 fresh:=make([]Event,0,len(items))
 for _,e:=range items{
  if _,exists:=s.seen[e.Identity];exists{continue}
  s.seen[e.Identity]=struct{}{}
  s.seenOrder=append(s.seenOrder,e.Identity)
  fresh=append(fresh,e)
  s.Items=append(s.Items,e)
 }
 sort.SliceStable(s.Items,func(i,j int)bool{return s.Items[i].Timestamp.Before(s.Items[j].Timestamp)})
 if len(s.Items)>s.Max{
  discard:=len(s.Items)-s.Max
  // Deliberately retain identities past visible-line eviction, because the
  // Dynatrace polling overlap will encounter old events again.
  s.Items=append([]Event(nil),s.Items[discard:]...)
 }
 for len(s.seenOrder)-s.seenHead>s.SeenMax{
  delete(s.seen,s.seenOrder[s.seenHead])
  s.seenHead++
 }
 if s.seenHead>10000 && s.seenHead*2>=len(s.seenOrder){
  s.seenOrder=append([]string(nil),s.seenOrder[s.seenHead:]...)
  s.seenHead=0
 }
 sort.SliceStable(fresh,func(i,j int)bool{return fresh[i].Timestamp.Before(fresh[j].Timestamp)})
 return fresh
}
func(s *Store)Services()[]string{
 m:=map[string]struct{}{}
 for _,e:=range s.Items{m[e.Service]=struct{}{}}
 keys:=make([]string,0,len(m))
 for k:=range m{keys=append(keys,k)}
 sort.Strings(keys)
 return keys
}
