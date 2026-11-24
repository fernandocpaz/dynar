package app

import (
 "context"
 "fmt"
 "net/url"
 "os"
 "os/exec"
 "regexp"
 "runtime"
 "strings"
 "time"

 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/tailg/pkg/logui"
)

func resourceBrowser(client *dynatrace.Client,cluster,namespace,timeFile string)logui.ResourceOpen{
 return func(ctx context.Context,workload,pod string)(string,error){
  now:=time.Now()
  from,to:=now.Add(-24*time.Hour),now
  if selection,ok:=logui.ReadSharedTimeRange(timeFile);ok&&selection.Lookback==0{from,to=selection.Bounds(now)}
  id,err:=client.ResourceEntity(ctx,namespace,cluster,workload,pod,from,to)
  if err!=nil{return "",err}
  link,err:=resourceURL(client.BaseURL,id,pod!="")
  if err!=nil{return "",err}
  if err=launchResourceBrowser(ctx,link);err!=nil{return "",err}
  name:=workload;if pod!=""{name=pod}
  return "Opened Dynatrace resources: "+name,nil
 }
}

var podEntityID=regexp.MustCompile(`^CLOUD_APPLICATION_INSTANCE-[0-9A-Fa-f]{16}$`)
var workloadEntityID=regexp.MustCompile(`^CLOUD_APPLICATION-[0-9A-Fa-f]{16}$`)

func resourceURL(base,id string,pod bool)(string,error){
 if err:=dynatrace.ValidateURL(base);err!=nil{return "",err}
 u,_:=url.Parse(base)
 if pod{
  if !podEntityID.MatchString(id){return "",fmt.Errorf("invalid Dynatrace pod entity ID")}
  u.Path="/ui/apps/dynatrace.kubernetes/resources/pod"
  q:=url.Values{};q.Set("entityId",id);u.RawQuery=q.Encode()
 }else{
  if !workloadEntityID.MatchString(id){return "",fmt.Errorf("invalid Dynatrace workload entity ID")}
  u.Path="/ui/nav/"+id
 }
 return u.String(),nil
}

func browserCommand(goos string,wsl bool,lookup func(string)(string,error),link string)(string,[]string,error){
 var candidates []struct{name string;args []string}
 switch goos{
 case "windows":candidates=append(candidates,struct{name string;args []string}{"rundll32.exe",[]string{"url.dll,FileProtocolHandler",link}})
 case "darwin":candidates=append(candidates,struct{name string;args []string}{"open",[]string{link}})
 default:
  if wsl{
   candidates=append(candidates,struct{name string;args []string}{"wslview",[]string{link}},struct{name string;args []string}{"rundll32.exe",[]string{"url.dll,FileProtocolHandler",link}})
  }
  candidates=append(candidates,struct{name string;args []string}{"xdg-open",[]string{link}},struct{name string;args []string}{"gio",[]string{"open",link}})
 }
 for _,candidate:=range candidates{if path,err:=lookup(candidate.name);err==nil{return path,candidate.args,nil}}
 return "",nil,fmt.Errorf("no supported browser launcher found")
}
func launchResourceBrowser(ctx context.Context,link string)error{
 version,_:=os.ReadFile("/proc/sys/kernel/osrelease")
 wsl:=os.Getenv("WSL_DISTRO_NAME")!=""||strings.Contains(strings.ToLower(string(version)),"microsoft")
 path,args,err:=browserCommand(runtime.GOOS,wsl,exec.LookPath,link)
 if err!=nil{return err}
 // Pass URLs as a single argument, never through a shell or cmd /c.
 if err=exec.CommandContext(ctx,path,args...).Run();err!=nil{return fmt.Errorf("open browser: %w",err)}
 return nil
}
