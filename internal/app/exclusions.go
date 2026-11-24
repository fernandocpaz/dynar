package app

import(
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "path/filepath"
 "sort"
)

// Keep window rules independent of the global text/time filter files.
// Canonicalize target sets so changing selection order never loses rules.
func windowExclusionFile(configPath string,opts SharedOptions,workloads,pods map[string]bool)string{
 type scope struct{
  Source,Cluster,Namespace string
  Kind string
  Targets []string
 }
 key:=scope{Source:opts.Source,Cluster:opts.Cluster,Namespace:opts.Namespace,Kind:"all"}
 if len(pods)>0{
  key.Kind="pods"
  for name,selected:=range pods{if selected{key.Targets=append(key.Targets,name)}}
 }else if len(workloads)>0{
  key.Kind="workloads"
  for name,selected:=range workloads{if selected{key.Targets=append(key.Targets,name)}}
 }
 sort.Strings(key.Targets)
 data,_:=json.Marshal(key)
 sum:=sha256.Sum256(data)
 return filepath.Join(configPath+".exclusions",hex.EncodeToString(sum[:])+".json")
}
