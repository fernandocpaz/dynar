package dynatrace

import (
 "context"
 "encoding/json"
 "fmt"
 "strconv"
 "strings"
 "time"
)

type PodState struct {
 Name, Ready, Phase, Reason, Node, Images string
 ConditionKnown, IsReady bool
 Restarts int
 RestartsKnown bool
 StartedAt, LastRestart time.Time
}

func NewInventoryClient(base string,credentials Credentials)(*Client,error){
 c,err:=NewClient(base,credentials)
 if err==nil{c.OAuthScopes="storage:buckets:read storage:smartscape:read"}
 return c,err
}

func PodInventoryDQL(namespace,cluster string,limit int)string{
 q:="smartscapeNodes K8S_POD\n| filter k8s.namespace.name == "+strconv.Quote(namespace)
 if cluster!=""{q+=" and k8s.cluster.name == "+strconv.Quote(cluster)}
 return q+"\n| parse k8s.object, \"JSON:obj\"\n| fields pod = k8s.pod.name, phase = k8s.pod.phase, node = k8s.node.name, status = obj[status], desired = arraySize(obj[spec][containers]), created = obj[metadata][creationTimestamp], deleted = obj[metadata][deletionTimestamp]\n| limit "+strconv.Itoa(limit)
}
type podCondition struct{Type,Status string}
type containerState struct{
 Waiting *struct{Reason string}
 Terminated *struct{Reason,FinishedAt string}
}
type containerStatus struct{
 Name,Image string
 Ready *bool
 RestartCount *int
 State,LastState containerState
}
type podStatus struct{
 Phase,StartTime string
 Conditions []podCondition
 ContainerStatuses []containerStatus
}
func str(v any)string{s,_:=v.(string);return s}
func parsedTime(s string)time.Time{t,_:=time.Parse(time.RFC3339Nano,s);return t}
func ParsePodInventory(records []map[string]any)map[string]PodState{
 out:=map[string]PodState{};duplicates:=map[string]bool{}
 for _,row:=range records{
  name:=str(row["pod"]);if name==""{continue}
  if _,ok:=out[name];ok{duplicates[name]=true;continue}
  p:=PodState{Name:name,Phase:str(row["phase"]),Node:str(row["node"]),StartedAt:parsedTime(str(row["created"]))}
  if str(row["deleted"])!=""{p.Phase="Terminating"}
  var status podStatus
  if raw,err:=json.Marshal(row["status"]);err==nil&&json.Unmarshal(raw,&status)==nil{
   if p.Phase==""{p.Phase=status.Phase}
   if p.StartedAt.IsZero(){p.StartedAt=parsedTime(status.StartTime)}
   for _,c:=range status.Conditions{if c.Type=="Ready"&&(c.Status=="True"||c.Status=="False"){p.ConditionKnown=true;p.IsReady=c.Status=="True"}}
   desired,known:=validMetricNumber(row["desired"])
   ready,reported:=0,0
   p.RestartsKnown=known&&desired>0&&float64(len(status.ContainerStatuses))==desired
   var images []string
   for _,c:=range status.ContainerStatuses{
    if c.Ready!=nil{reported++;if *c.Ready{ready++}}
    if c.RestartCount!=nil&&*c.RestartCount>=0{
     p.Restarts+=*c.RestartCount
     if *c.RestartCount>0&&c.LastState.Terminated!=nil{
      t:=parsedTime(c.LastState.Terminated.FinishedAt);if t.After(p.LastRestart){p.LastRestart=t}
     }
    }else{p.RestartsKnown=false}
    if c.Image!=""{images=append(images,c.Image)}
    if p.Reason==""{
     if c.State.Waiting!=nil{p.Reason=c.State.Waiting.Reason}
     if c.State.Terminated!=nil&&c.State.Terminated.Reason!="Completed"{p.Reason=c.State.Terminated.Reason}
    }
   }
   if known&&desired>0&&float64(reported)==desired{p.Ready=fmt.Sprintf("%d/%d",ready,int(desired))}
   p.Images=strings.Join(images,", ")
  }
  if p.Phase=="Terminating"{p.ConditionKnown=true;p.IsReady=false}
  if p.ConditionKnown{
   label:="Ready";if !p.IsReady{label="NotReady"}
   if p.Ready!=""{p.Ready+=" "};p.Ready+=label
  }else if p.Ready!=""{p.Ready+=" ?"}
  out[name]=p
 }
 for name:=range duplicates{delete(out,name)}
 return out
}
func(c *Client)QueryPodInventory(ctx context.Context,namespace,cluster string)(map[string]PodState,error){
 if namespace==""{return nil,fmt.Errorf("namespace required")}
 const limit=3000
 now:=time.Now().UTC()
 r,err:=c.QueryDQL(ctx,PodInventoryDQL(namespace,cluster,limit),now.Add(-5*time.Minute),now,limit)
 if err!=nil{return nil,err}
 if r.AtLimit||len(r.Warnings)>0{return nil,fmt.Errorf("pod inventory incomplete")}
 return ParsePodInventory(r.Records),nil
}

func(c *Client)QueryPodMetrics(ctx context.Context,namespace,cluster string)(map[string]WorkloadMetrics,error){
 const limit=3000
 // Match the established CPU and memory query, grouped by exact pod and cluster.
 q:=strings.ReplaceAll(MetricsDQL(namespace,cluster,limit),"k8s.workload.name","k8s.pod.name")
 q=strings.Replace(q,"by: {k8s.namespace.name, k8s.pod.name}","by: {k8s.namespace.name, k8s.cluster.name, k8s.pod.name}",1)
 now:=time.Now().UTC()
 r,err:=c.QueryDQL(ctx,q,now.Add(-3*time.Minute),now,limit)
 if err!=nil{return nil,err}
 if r.AtLimit||len(r.Warnings)>0{return nil,fmt.Errorf("pod metrics incomplete")}
 out:=map[string]WorkloadMetrics{};duplicates:=map[string]bool{}
 for _,row:=range r.Records{
  name:=str(row["k8s.pod.name"]);if name==""{continue}
  if _,ok:=out[name];ok{duplicates[name]=true;continue}
  m:=WorkloadMetrics{Name:name}
  m.CPUPercent,m.CPUKnown=latestRatio(metricValues(row,"usage_cpu"),metricValues(row,"limit_cpu"))
  m.MemoryBytes,m.MemoryKnown=latestMetric(metricValues(row,"memory"))
  m.MemoryLimitBytes,m.MemoryLimitKnown=latestMetric(metricValues(row,"memory_limit"))
  out[name]=m
 }
 for name:=range duplicates{delete(out,name)}
 return out,nil
}
