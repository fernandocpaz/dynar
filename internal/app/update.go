package app

import (
    "context"
    "flag"
    "fmt"
    "io"
    "os"
    "strings"
    "time"

    "github.com/fernandocpaz/dynar/internal/update"
)

func updateKnown(v string)bool{return update.KnownVersion(v)}
func isUpdateCheckDisabled()bool {
    v:=strings.ToLower(strings.TrimSpace(os.Getenv("DYNAR_NO_UPDATE_CHECK")))
    return v=="1"||v=="true"||v=="yes"
}
func updateCommand(ctx context.Context,args []string,stdout,stderr io.Writer)int{
    flags:=flag.NewFlagSet("update",flag.ContinueOnError)
    flags.SetOutput(stderr)
    checkOnly:=flags.Bool("check",false,"Check for updates without installing")
    if err:=flags.Parse(args);err!=nil{return 2}
    if flags.NArg()!=0{fmt.Fprintln(stderr,"usage: dynar update [--check]");return 2}
    client:=update.New(InstalledVersion())
    client.Cleanup()
    limited,cancel:=context.WithTimeout(ctx,12*time.Second)
    release,err:=client.Check(limited)
    cancel()
    if err!=nil {
        fmt.Fprintln(stderr,"Unable to check for DynaR updates:",err)
        fmt.Fprintln(stderr,"See https://github.com/fernandocpaz/dynar/releases/latest")
        return 1
    }
    fmt.Fprintf(stdout,"Installed: %s  Latest: %s\n",client.Version,release.Version)
    if *checkOnly{return 0}
    if update.KnownVersion(client.Version)&&!update.Newer(client.Version,release.Version){
        fmt.Fprintln(stdout,"Already up to date; no changes made.")
        return 0
    }
    if err:=client.Install(ctx,release);err!=nil{
        fmt.Fprintf(stderr,"Update failed: %v\nManual install: go install github.com/fernandocpaz/dynar/cmd/dynar@%s\n",err,release.Version)
        return 1
    }
    fmt.Fprintf(stdout,"Updated to %s. Launch DynaR again to use the new version.\n",release.Version)
    return 0
}
