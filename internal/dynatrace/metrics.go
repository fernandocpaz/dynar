package dynatrace

import (
 "context"
 "fmt"
 "math"
 "strconv"
 "strings"
 "time"
)

// WorkloadMetrics are *observed* Grail metrics for a namespace workload.
// CPUPercent is percentage of the workload's configured CPU limit; never
// assume the raw CPU usage metric is millicores.
type WorkloadMetrics struct {
 Name string
 CPUPercent float64
 CPUKnown bool
 MemoryBytes float64
 MemoryKnown bool
 MemoryLimitBytes float64
 MemoryLimitKnown bool
}
type MetricsSnapshot struct {
 Workloads []WorkloadMetrics
 Warnings []string
}

// MetricsDQL returns a single namespace-scoped Grail query. Dynatrace's own
// alerting examples recommend CPU usage / CPU limit and memory working set.
// Absence of a limit produces NULL, never a manufactured zero usage percent.
func MetricsDQL(namespace,cluster string,limit int)string{
 filter:="k8s.namespace.name == "+strconv.Quote(namespace)
 if cluster!=""{filter+=" and k8s.cluster.name == "+strconv.Quote(cluster)}
 return `timeseries {
  usage_cpu = sum(dt.kubernetes.container.cpu_usage, rollup: avg),
  limit_cpu = sum(dt.kubernetes.container.limits_cpu, rollup: avg),
  memory = sum(dt.kubernetes.container.memory_working_set, rollup: avg),
  memory_limit = sum(dt.kubernetes.container.limits_memory, rollup: avg)
}, by: {k8s.namespace.name, k8s.workload.name},
 filter: {`+filter+`}, interval: 1m
| fields k8s.workload.name, usage_cpu, limit_cpu, memory, memory_limit
| limit `+strconv.Itoa(limit)
}
func validMetricNumber(raw any)(float64,bool){
 var value float64
 switch x:=raw.(type){
 case float64:value=x
 case float32:value=float64(x)
 case int:value=float64(x)
 case int64:value=float64(x)
 default:return 0,false
 }
 return value,!math.IsNaN(value)&&!math.IsInf(value,0)&&value>=0
}
func metricValues(record map[string]any,name string)[]any{
 if values,ok:=record[name].([]any);ok{return values}
 // The standard Grail SDK uses []any after JSON unmarshalling. Supporting
 // typed arrays also makes fake metrics providers and tests straightforward.
 if values,ok:=record[name].([]float64);ok{
  out:=make([]any,len(values))
  for i,v:=range values{out[i]=v}
  return out
 }
 return nil
}
func latestMetric(values []any)(float64,bool){
 for i:=len(values)-1;i>=0;i--{
  if n,ok:=validMetricNumber(values[i]);ok{return n,true}
 }
 return 0,false
}
func latestRatio(usage,limit []any)(float64,bool){
 end:=len(usage)
 if len(limit)<end{end=len(limit)}
 for i:=end-1;i>=0;i--{
  used,a:=validMetricNumber(usage[i])
  max,b:=validMetricNumber(limit[i])
  if a&&b&&max>0{return 100*used/max,true}
 }
 return 0,false
}
func ParseMetrics(records []map[string]any)([]WorkloadMetrics,[]string){
 result:=make([]WorkloadMetrics,0,len(records))
 warning:=[]string{}
 seen:=make(map[string]bool,len(records))
 for _,record:=range records{
  name,ok:=record["k8s.workload.name"].(string)
  if !ok||strings.TrimSpace(name)==""{continue}
  if seen[name]{
   // Duplicate workloads across clusters must not silently replace one
   // another; request cluster scoping to avoid ambiguity.
   warning=append(warning,fmt.Sprintf("duplicate metrics for %q: use --cluster to disambiguate",name))
   continue
  }
  seen[name]=true
  item:=WorkloadMetrics{Name:name}
  item.CPUPercent,item.CPUKnown=latestRatio(metricValues(record,"usage_cpu"),metricValues(record,"limit_cpu"))
  item.MemoryBytes,item.MemoryKnown=latestMetric(metricValues(record,"memory"))
  item.MemoryLimitBytes,item.MemoryLimitKnown=latestMetric(metricValues(record,"memory_limit"))
  result=append(result,item)
 }
 return result,warning
}
func(c *Client)QueryWorkloadMetrics(ctx context.Context,namespace,cluster string)(MetricsSnapshot,error){
 if namespace==""{return MetricsSnapshot{},fmt.Errorf("namespace required for metrics")}
 const limit=3000
 end:=time.Now().UTC()
 result,err:=c.QueryDQL(ctx,MetricsDQL(namespace,cluster,limit),end.Add(-5*time.Minute),end,limit)
 if err!=nil{return MetricsSnapshot{},err}
 records,warnings:=ParseMetrics(result.Records)
 warnings=append(warnings,result.Warnings...)
 if result.AtLimit{warnings=append(warnings,"metrics query limit reached; workload metrics may be incomplete")}
 if len(records)==0{warnings=append(warnings,"no Grail CPU/memory metrics observed in this namespace (check storage:metrics:read and Kubernetes metric ingestion)")}
 return MetricsSnapshot{Workloads:records,Warnings:warnings},nil
}
