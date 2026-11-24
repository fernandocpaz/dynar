package app

import (
 "context"
 "fmt"
 "io"
 "sort"
 "strconv"
 "strings"
 "sync"
 "time"
 "unicode"

 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/tailg/pkg/logui"
)

const scopeLimit=2000
const scopeTTL=2*time.Minute
type scopeQuery func(context.Context,string,time.Time,time.Time,int)(dynatrace.Result,error)
type scopeCacheEntry struct{at time.Time;names []string;warning string}
type scopeCatalog struct{
 mu sync.Mutex
 query scopeQuery
 timeFile string
 cache map[string]scopeCacheEntry
 recent map[string][]string
}
func newScopeCatalog(client *dynatrace.Client,timeFile string)*scopeCatalog{
 c:=&scopeCatalog{timeFile:timeFile,cache:make(map[string]scopeCacheEntry),recent:make(map[string][]string)}
 if client!=nil{c.query=client.QueryDQL}
 return c
}
func scopeKey(kind,cluster string)string{if kind=="cluster"{return "cluster"};return "namespace:"+cluster}
func(c *scopeCatalog)remember(cluster,namespace string){
 c.mu.Lock();defer c.mu.Unlock()
 for key,value:=range map[string]string{"cluster":cluster,scopeKey("namespace",cluster):namespace}{
  names:=[]string{value}
  for _,name:=range c.recent[key]{if name!=value&&len(names)<12{names=append(names,name)}}
  c.recent[key]=names
 }
}
func scopeDQL(kind,cluster string)(string,string,error){
 field:="k8s.namespace.name"
 if kind=="cluster"{field="k8s.cluster.name"}else if kind!="namespace"{return "","",fmt.Errorf("invalid scope type")}
 query:="fetch logs"
 if kind=="namespace"&&cluster!=""{query+="\n| filter k8s.cluster.name == "+strconv.Quote(cluster)}
 query+="\n| filter isNotNull("+field+")\n| fields "+field+"\n| dedup "+field+"\n| sort "+field+" asc\n| limit "+strconv.Itoa(scopeLimit)
 return query,field,nil
}
func rankScope(recent,names []string,current string,allowEmpty bool)[]string{
 seen:=map[string]bool{}
 var result []string
 add:=func(name string){
  if !seen[name]&&(name!=""||allowEmpty){seen[name]=true;result=append(result,name)}
 }
 add(current)
 for _,name:=range recent{add(name)}
 names=append([]string(nil),names...)
 sort.Strings(names)
 for _,name:=range names{add(name)}
 if allowEmpty{add("")}
 return result
}
func(c *scopeCatalog)list(ctx context.Context,kind,cluster,namespace string)([]string,string,error){
 dql,field,err:=scopeDQL(kind,cluster);if err!=nil{return nil,"",err}
 key:=scopeKey(kind,cluster)
 current:=namespace;if kind=="cluster"{current=cluster}
 now:=time.Now()
 from,to:=now.Add(-24*time.Hour),now
 if selection,ok:=logui.ReadSharedTimeRange(c.timeFile);ok&&selection.Lookback==0{from,to=selection.Bounds(now);key+=":"+from.Format(time.RFC3339Nano)+":"+to.Format(time.RFC3339Nano)}
 c.mu.Lock()
 entry,hit:=c.cache[key]
 recent:=append([]string(nil),c.recent[scopeKey(kind,cluster)]...)
 c.mu.Unlock()
 rank:=func(names []string)[]string{return rankScope(recent,names,current,kind=="cluster")}
 if hit&&now.Sub(entry.at)<scopeTTL{return rank(entry.names),entry.warning,nil}
 if c.query==nil{return rank(nil),"",fmt.Errorf("Dynatrace discovery is not configured")}
 result,err:=c.query(ctx,dql,from,to,scopeLimit)
 if err!=nil{return rank(nil),"",err}
 if err=ctx.Err();err!=nil{return nil,"",err}
 var names []string
 for _,row:=range result.Records{if name,ok:=row[field].(string);ok&&validScopeName(name,false){names=append(names,name)}}
 warning:=strings.Join(result.Warnings,"; ")
 if result.AtLimit{if warning!=""{warning+="; "};warning+="Discovery may be incomplete"}
 c.mu.Lock()
 // Keep discovery memory bounded even when changing fixed ranges frequently.
 if len(c.cache)>=64{c.cache=make(map[string]scopeCacheEntry)}
 c.cache[key]=scopeCacheEntry{at:now,names:names,warning:warning}
 c.mu.Unlock()
 return rank(names),warning,nil
}
func validScopeName(name string,allowEmpty bool)bool{
 if name==""{return allowEmpty}
 if len(name)>253||strings.TrimSpace(name)!=name{return false}
 for _,r:=range name{if unicode.IsControl(r){return false}}
 return true
}
func changeScope(opts SharedOptions,kind,value string)(SharedOptions,error){
 value=strings.TrimSpace(value)
 switch kind{
 case "cluster":
  if !validScopeName(value,true){return opts,fmt.Errorf("invalid cluster name")}
  opts.Cluster=value
 case "namespace":
  if !validScopeName(value,false){return opts,fmt.Errorf("invalid namespace name")}
  opts.Namespace=value
 default:return opts,fmt.Errorf("invalid scope type")
 }
 // Workload selections and synchronized text filters belong to the old scope.
 opts.FilterFile=""
 return opts,nil
}
func pollScope(parent context.Context,b *Broker)(context.Context,func()){
 ctx,cancel:=context.WithCancel(parent)
 done:=make(chan struct{})
 go func(){defer close(done);b.Run(ctx)}()
 return ctx,func(){cancel();<-done}
}
func startScope(parent context.Context,c Collector,opts SharedOptions,timeFile string,stderr io.Writer)(context.Context,func(),*Broker,error){
 b:=NewBroker(c,opts.Source,opts.Namespace,opts.Cluster,opts.Since,opts.Refresh,opts.BufferLines)
 b.timeFile=timeFile
 err:=withLoading(stderr,loadingNamespaceMessage(opts),func()error{return b.Bootstrap(parent)})
 if status:=b.Status();status!=nil{fmt.Fprintln(stderr,"WARNING: Dynatrace returned incomplete history:",status)}
 ctx,stop:=pollScope(parent,b)
 return ctx,stop,b,err
}
