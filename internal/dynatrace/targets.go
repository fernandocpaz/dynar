package dynatrace

import (
 "context"
 "strconv"
 "strings"
 "time"
)

// targetPredicate narrows historical queries to the selected exact sources,
// while retaining records that only carry deployment, container, or pod labels.
// The adapter still checks the parsed workload/pod locally.
func targetPredicate(workloads,pods []string)string{
 var parts []string
 if len(pods)>0{
  for _,pod:=range pods{
   // The broker synthesizes these names when Grail has no pod attribute.
   // An exact server-side pod condition would silently omit those records.
   if strings.HasSuffix(pod,"-unknown"){return ""}
   parts=append(parts,"k8s.pod.name == "+strconv.Quote(pod))
  }
  return strings.Join(parts," or ")
 }
 for _,name:=range workloads{
  quoted:=strconv.Quote(name)
  for _,field:=range []string{"k8s.workload.name","k8s.deployment.name","k8s.statefulset.name","k8s.daemonset.name","k8s.job.name","k8s.cronjob.name","k8s.container.name","dt.process.name","k8s.pod.name"}{
   parts=append(parts,field+" == "+quoted)
  }
  parts=append(parts,"startsWith(k8s.pod.name, "+strconv.Quote(name+"-")+")")
 }
 return strings.Join(parts," or ")
}
func(c *Client)QueryTargets(ctx context.Context,namespace,cluster string,start,end time.Time,limit int,workloads,pods []string)(Result,error){
 return c.QueryDQL(ctx,scopedLogsDQL(namespace,cluster,limit,workloads,pods),start,end,limit)
}
