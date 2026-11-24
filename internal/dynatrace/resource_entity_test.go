package dynatrace
import(
 "strings"
 "testing"
)
func TestResourceEntityQueryAndAmbiguity(t *testing.T){
 q,field,err:=resourceEntityDQL("demo-shop","example-production","checkout-api","checkout-api-123")
 if err!=nil||field!="dt.entity.cloud_application_instance"||!strings.Contains(q,`k8s.pod.name == "checkout-api-123"`)||!strings.Contains(q,`k8s.cluster.name == "example-production"`){t.Fatalf("%s %s %v",q,field,err)}
 first:="CLOUD_APPLICATION_INSTANCE-1234567890ABCDEF"
 second:="CLOUD_APPLICATION_INSTANCE-1234567890ABCDE0"
 result:=Result{Records:[]map[string]any{{field:first}}}
 id,err:=resourceEntityID(result,field)
 if err!=nil||id!=first{t.Fatalf("%q %v",id,err)}
 result.Records=append(result.Records,map[string]any{field:second})
 if _,err=resourceEntityID(result,field);err==nil{t.Fatal("ambiguous entities accepted")}
 if _,err=resourceEntityID(Result{},field);err==nil{t.Fatal("missing entity accepted")}
 q,field,err=resourceEntityDQL("demo-shop","","checkout-api","")
 if err!=nil||field!="dt.entity.cloud_application"||!strings.Contains(q,`k8s.workload.name == "checkout-api"`)||strings.Contains(q,"k8s.cluster.name"){t.Fatal(q)}
}
