package app

import (
 "fmt"
 "os"
 "os/exec"
 "runtime"
 "strconv"
 "strings"
)

// SplitPaneArgs follows tailg's alternating Windows Terminal pane layout.
// Per-pane DynaR processes query the same Grail namespace independently,
// so the regular single-process view is preferable for large selections.
func SplitPaneArgs(workloads []string,opts SharedOptions,executable string)[]string{
 if len(workloads)==0{return nil}
 child:=func(workload string)[]string{
  args:=[]string{executable,"-n",opts.Namespace,"--service",workload,"--service-exact","--no-update-check"}
  if opts.Source!="" && opts.Source!="default"{args=append(args,"--source",opts.Source)}
  if opts.Cluster!=""{args=append(args,"--cluster",opts.Cluster)}
  if opts.Since>0{args=append(args,"--since",opts.Since.String())}
  if opts.Refresh>0{args=append(args,"--refresh",opts.Refresh.String())}
  if opts.BufferLines>0{args=append(args,"--buffer-lines",strconv.Itoa(opts.BufferLines))}
  if opts.Search!=""{args=append(args,"--search",opts.Search)}
  if opts.FilterFile!=""{args=append(args,"--filter-file",opts.FilterFile)}
  if opts.Regex{args=append(args,"--regex")}
  if opts.Level!=""{args=append(args,"--level",opts.Level)}
  return args
 }
 args:=[]string{"new-tab","--title","DynaR "+workloads[0],"--"}
 args=append(args,child(workloads[0])...)
 vertical:=true
 for i,workload:=range workloads[1:]{
  direction:="-V"
  if !vertical{direction="-H"}
  vertical=!vertical
  remaining:=len(workloads)-i
  size:=float64(remaining-1)/float64(remaining)
  sz:=strings.TrimRight(strings.TrimRight(strconv.FormatFloat(size,'f',6,64),"0"),".")
  args=append(args,";","split-pane",direction,"--size",sz,"--title","DynaR "+workload,"--")
  args=append(args,child(workload)...)
 }
 return args
}
func LaunchSplitPanes(workloads []string,opts SharedOptions)error{
 if runtime.GOOS!="windows"{return fmt.Errorf("DynaR split panes require Windows Terminal on Windows")}
 if len(workloads)==0{return fmt.Errorf("no workloads selected for split panes")}
 executable,err:=os.Executable()
 if err!=nil{return fmt.Errorf("resolve DynaR executable: %w",err)}
 if _,err:=exec.LookPath("wt");err!=nil{return fmt.Errorf("Windows Terminal wt.exe is not available in PATH: %w",err)}
 args:=SplitPaneArgs(workloads,opts,executable)
 if err:=exec.Command("wt",args...).Start();err!=nil{return fmt.Errorf("launch Windows Terminal panes: %w",err)}
 return nil
}
