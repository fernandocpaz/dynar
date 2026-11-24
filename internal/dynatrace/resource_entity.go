package dynatrace

import(
 "context"
 "fmt"
 "regexp"
 "strconv"
 "strings"
 "time"
)

func resourceEntityDQL(namespace,cluster,workload,pod string)(string,string,error){
 if strings.TrimSpace(namespace)==""||strings.TrimSpace(workload)==""&&strings.TrimSpace(pod)==""{return "","",fmt.Errorf("missing resource scope")}
 field:="dt.entity.cloud_application"
 predicate:=""
 if pod!=""{
  field="dt.entity.cloud_application_instance"
  predicate="k8s.pod.name == "+strconv.Quote(pod)
 }else{
  predicate=targetPredicate([]string{workload},nil)
 }
 q:="fetch logs\n| filter k8s.namespace.name == "+strconv.Quote(namespace)
 if cluster!=""{q+="\n| filter k8s.cluster.name == "+strconv.Quote(cluster)}
 q+="\n| filter ("+predicate+")\n| filter isNotNull("+field+")\n| fields "+field+"\n| dedup "+field+"\n| limit 2"
 return q,field,nil
}

func(c *Client)ResourceEntity(ctx context.Context,namespace,cluster,workload,pod string,from,to time.Time)(string,error){
 q,field,err:=resourceEntityDQL(namespace,cluster,workload,pod)
 if err!=nil{return "",err}
 result,err:=c.QueryDQL(ctx,q,from,to,2)
 if err!=nil{return "",err}
 return resourceEntityID(result,field)
}

var resourcePodID=regexp.MustCompile(`^CLOUD_APPLICATION_INSTANCE-[0-9A-Fa-f]{16}$`)
var resourceWorkloadID=regexp.MustCompile(`^CLOUD_APPLICATION-[0-9A-Fa-f]{16}$`)
func resourceEntityID(result Result,field string)(string,error){
 valid:=resourceWorkloadID
 if field=="dt.entity.cloud_application_instance"{valid=resourcePodID}
 ids:=map[string]bool{}
 add:=func(v any){if id,ok:=v.(string);ok&&valid.MatchString(id){ids[id]=true}}
 for _,row:=range result.Records{
  switch v:=row[field].(type){
  case []any:for _,item:=range v{add(item)}
  case []string:for _,item:=range v{add(item)}
  default:add(v)
  }
 }
 if len(ids)>1||result.AtLimit{return "",fmt.Errorf("multiple resource entities or incomplete results; select a specific cluster or narrower time range")}
 if len(result.Warnings)>0{return "",fmt.Errorf("entity resolution incomplete: %s",strings.Join(result.Warnings,"; "))}
 for id:=range ids{return id,nil}
 return "",fmt.Errorf("no Dynatrace entity ID in matching logs; Kubernetes entity enrichment may be unavailable")
}
