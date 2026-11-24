package app

import (
 "fmt"
 "os"
 "os/exec"
 "runtime"
 "strconv"
 "strings"
)

// SplitPaneArgs retains tailg's alternating Windows Terminal layout and
// launches one exact workload per child.
func SplitPaneArgs(workloads []string,opts SharedOptions,executable string)[]string{
 return splitPaneArgs(workloads,opts,executable,false)
}

// SplitPodPaneArgs launches one Grail pod per pane. It must not use the
// workload filter, which could include multiple replicas.
func SplitPodPaneArgs(pods []string,opts SharedOptions,executable string)[]string{
 return splitPaneArgs(pods,opts,executable,true)
}

func splitPaneArgs(names []string,opts SharedOptions,executable string,podMode bool)[]string{
 if len(names)==0{return nil}
 child:=func(name string)[]string{
  args:=[]string{executable,"-n",opts.Namespace}
  if podMode{
   args=append(args,"--pod-exact",name)
  }else{
   args=append(args,"--service",name,"--service-exact")
  }
  args=append(args,"--no-update-check")
  if opts.Source!=""&&opts.Source!="default"{args=append(args,"--source",opts.Source)}
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
 args:=[]string{"new-tab","--title","DynaR "+names[0],"--"}
 args=append(args,child(names[0])...)
 vertical:=true
 for i,name:=range names[1:]{
  direction:="-V"
  if !vertical{direction="-H"}
  vertical=!vertical
  remaining:=len(names)-i
  size:=float64(remaining-1)/float64(remaining)
  sz:=strings.TrimRight(strings.TrimRight(strconv.FormatFloat(size,'f',6,64),"0"),".")
  args=append(args,";","split-pane",direction,"--size",sz,"--title","DynaR "+name,"--")
  args=append(args,child(name)...)
 }
 return args
}

func LaunchSplitPanes(workloads []string,opts SharedOptions)error{
 return launchSplitPanes(workloads,opts,false)
}
func LaunchPodSplitPanes(pods []string,opts SharedOptions)error{
 return launchSplitPanes(pods,opts,true)
}
func launchSplitPanes(names []string,opts SharedOptions,podMode bool)error{
 if runtime.GOOS!="windows"{return fmt.Errorf("DynaR split panes require Windows Terminal on Windows")}
 if len(names)==0{return fmt.Errorf("no workloads or pods selected for split panes")}
 executable,err:=os.Executable()
 if err!=nil{return fmt.Errorf("resolve DynaR executable: %w",err)}
 if _,err:=exec.LookPath("wt");err!=nil{return fmt.Errorf("Windows Terminal wt.exe is not available in PATH: %w",err)}
 args:=splitPaneArgs(names,opts,executable,podMode)
 if err:=exec.Command("wt",args...).Start();err!=nil{return fmt.Errorf("launch Windows Terminal panes: %w",err)}
 return nil
}
