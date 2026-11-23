package app

import (
    "fmt"
    "runtime/debug"
    "strings"
)

// Release builds override these via -ldflags; go install also includes
// module version metadata through debug.ReadBuildInfo.
var (
    Version = "dev"
    Commit string
    BuildTime string
)
const RestartExitCode = 75

func InstalledVersion() string {
    if Version!="" && Version!="dev" {return Version}
    if info,ok:=debug.ReadBuildInfo();ok{
        if v:=info.Main.Version;v!="" && v!="(devel)"{return v}
    }
    return "dev"
}
func StableBuild() bool {
    version:=InstalledVersion()
    return updateKnown(version) && !strings.ContainsAny(version,"-+")
}
func BuildDescription()string {
    s:="DynaR "+InstalledVersion()
    if Commit!=""{
        short:=Commit
        if len(short)>12{short=short[:12]}
        s+=fmt.Sprintf(" commit %s",short)
    }
    if BuildTime!=""{s+=" built "+BuildTime}
    return s
}
