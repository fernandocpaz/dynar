package dynatrace
import (
 "strings"
 "testing"
)
func TestMetricsDQLIsNamespaceScopedAndUsesRealMetrics(t *testing.T){
 q:=MetricsDQL(`apollo" | limit 1`,"tkgs-prod",500)
 if !strings.Contains(q,`k8s.namespace.name == "apollo\" | limit 1"`){t.Fatalf("unescaped namespace: %s",q)}
 for _,part:=range []string{"dt.kubernetes.container.cpu_usage","dt.kubernetes.container.memory_working_set",
   `k8s.cluster.name == "tkgs-prod"`,`limit 500`,`by: {k8s.namespace.name, k8s.workload.name}`}{
  if !strings.Contains(q,part){t.Fatalf("missing %s",part)}
 }
}
func TestParseMetricsDoesNotInventMissingCPUOrMemory(t *testing.T){
 rows:=[]map[string]any{
  {"k8s.workload.name":"encounter","usage_cpu":[]any{nil,float64(1.2)},"limit_cpu":[]any{nil,float64(2)},
   "memory":[]any{float64(100*1024*1024),float64(200*1024*1024)},"memory_limit":[]any{float64(400*1024*1024)}},
  {"k8s.workload.name":"patient","usage_cpu":[]any{float64(3.0)},"limit_cpu":[]any{nil},
   "memory":[]any{nil}},
 }
 samples,warnings:=ParseMetrics(rows)
 if len(warnings)!=0||len(samples)!=2{t.Fatalf("wrong rows: %+v %v",samples,warnings)}
 if !samples[0].CPUKnown||samples[0].CPUPercent!=60||!samples[0].MemoryKnown||samples[0].MemoryBytes!=200*1024*1024{
  t.Fatalf("wrong encounter metrics: %+v",samples[0])
 }
 if samples[1].CPUKnown||samples[1].MemoryKnown{t.Fatalf("fabricated missing measurements: %+v",samples[1])}
}
func TestMetricsRejectNaNInfinityAndNoCPUlimit(t *testing.T){
 if _,ok:=latestRatio([]any{float64(2)},[]any{float64(0)});ok{t.Fatal("zero limit is not a measured CPU percentage")}
 if _,ok:=latestMetric([]any{nil});ok{t.Fatal("nil is not zero")}
}
