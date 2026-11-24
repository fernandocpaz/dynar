package app
import(
 "fmt"
 "strings"
 "testing"
)
func TestResourceBrowserURL(t *testing.T){
 base:="https://example.apps.dynatrace.com"
 pod:="CLOUD_APPLICATION_INSTANCE-1234567890ABCDEF"
 got,err:=resourceURL(base,pod,true)
 if err!=nil||got!=base+"/ui/apps/dynatrace.kubernetes/resources/pod?entityId="+pod{t.Fatalf("%s %v",got,err)}
 workload:="CLOUD_APPLICATION-1234567890ABCDEF"
 got,err=resourceURL(base,workload,false)
 if err!=nil||got!=base+"/ui/nav/"+workload{t.Fatalf("%s %v",got,err)}
 if _,err=resourceURL(base,workload,true);err==nil{t.Fatal("accepted wrong entity type")}
 if _,err=resourceURL("https://user:secret@example.com",pod,true);err==nil{t.Fatal("accepted credentials in URL")}
}
func TestResourceBrowserUsesDirectArguments(t *testing.T){
 lookup:=func(name string)(string,error){if name=="rundll32.exe"{return name,nil};return "",fmt.Errorf("missing")}
 link:="https://example.apps.dynatrace.com/ui/apps/dynatrace.kubernetes/resources/pod?entityId=CLOUD_APPLICATION_INSTANCE-1234567890ABCDEF"
 for _,platform:=range []string{"windows","linux"}{
  path,args,err:=browserCommand(platform,true,lookup,link)
  if err!=nil||!strings.HasSuffix(path,"rundll32.exe")||len(args)!=2||args[1]!=link{t.Fatalf("%s %v %v",path,args,err)}
 }
}
