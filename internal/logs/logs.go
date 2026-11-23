package logs

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "sort"
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
 RequestID string `json:"request_id,omitempty"`
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
  RequestID:stringField(rec,"request_id","request.id"),
 }
 if e.Service==""{e.Service=WorkloadFromPod(e.Pod)}
 if e.Service==""{e.Service="unknown"}
 if e.Level==""{e.Level=GuessLevel(e.Message)}
 if e.Message==""{e.Message="(empty message)"}
 native:=stringField(rec,"id","event.id")
 if native!=""{e.Identity=source+"/"+native}else{
  hash:=sha256.Sum256([]byte(e.Timestamp.UTC().Format(time.RFC3339Nano)+"\x00"+source+"\x00"+e.Pod+"\x00"+e.Container+"\x00"+e.Message+"\x00"+e.TraceID))
  e.Identity=hex.EncodeToString(hash[:])
 }
 return e,nil
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
