package app

import (
 "fmt"
 "io"
 "os"
 "strings"
 "time"
 "unicode/utf8"

 "golang.org/x/term"
)

const waitFrames="|/-\\"

// withLoading leaves redirected output free of carriage-return animation and
// stops its only writer before returning control to the caller.
func withLoading(out io.Writer,label string,work func()error)error{
 f,ok:=out.(*os.File)
 animate:=false
 if ok&&term.IsTerminal(int(f.Fd()))&&os.Getenv("TERM")!="dumb"{
  width,_,err:=term.GetSize(int(f.Fd()))
  animate=err==nil&&width>utf8.RuneCountInString(label)+24
 }
 return runLoading(out,label,animate,work)
}

func runLoading(out io.Writer,label string,animate bool,work func()error)error{
 if !animate{
  fmt.Fprintln(out,label)
  return work()
 }
 start:=time.Now()
 done,stopped:=make(chan struct{}),make(chan struct{})
 go func(){
  defer close(stopped)
  ticker:=time.NewTicker(125*time.Millisecond)
  defer ticker.Stop()
  frame:=0
  draw:=func(){
   fmt.Fprintf(out,"\r[%c] %s  %s",waitFrames[frame%len(waitFrames)],label,time.Since(start).Truncate(time.Second))
   frame++
  }
  draw()
  for{
   select{
   case <-done:return
   case <-ticker.C:draw()
   }
  }
 }()
 err:=work()
 close(done)
 <-stopped
 result:="done"
 if err!=nil{result="failed"}
 fmt.Fprintf(out,"\r[%s] %s (%s)%s\n",result,label,time.Since(start).Round(time.Millisecond),strings.Repeat(" ",8))
 return err
}
