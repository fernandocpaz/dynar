package main

import (
 "context"
 "fmt"
 "os"
 "os/signal"
 "syscall"
 "github.com/fernandocpaz/dynar/internal/app"
)

func main() {
 ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
 defer stop()
 executable,_:=os.Executable()
 code:=app.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
 if code==app.RestartExitCode {
  stop()
  if err:=app.Restart(executable,os.Args[1:]);err!=nil{
   fmt.Fprintln(os.Stderr,"DynaR restarted failed:",err)
   os.Exit(1)
  }
  return
 }
 os.Exit(code)
}
