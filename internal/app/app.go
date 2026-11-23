package app

import (
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "os"
 "regexp"
 "sort"
 "strings"
 "time"
 "github.com/fernandocpaz/dynar/internal/config"
 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/dynar/internal/logs"
 )


func Run(ctx context.Context,args []string,stdout,stderr io.Writer)int{
 if len(args)>0{
  switch args[0]{
   case "help","--help","-h":usage(stdout);return 0
   case "version","--version":fmt.Fprintln(stdout,BuildDescription());return 0
   case "update":return updateCommand(ctx,args[1:],stdout,stderr)
   case "source":return sources(args[1:],stdout,stderr)
  }
 }
 fs:=flag.NewFlagSet("dynar",flag.ContinueOnError)
 fs.SetOutput(stderr)
 namespace:=fs.String("n","","Kubernetes namespace (required)")
 nsLong:=fs.String("namespace","","Kubernetes namespace")
 source:=fs.String("source","","Configured Dynatrace source")
 cluster:=fs.String("cluster","","Optional Kubernetes cluster name")
 since:=fs.Duration("since",30*time.Minute,"Initial lookback")
 refresh:=fs.Duration("refresh",10*time.Second,"Follow polling interval")
 once:=fs.Bool("once",false,"Query history once without following")
 noFollow:=fs.Bool("no-follow",false,"Alias of --once")
 search:=fs.String("search","","Search content and metadata")
 regex:=fs.Bool("regex",false,"Treat --search as regex")
 level:=fs.String("level","","Filter by level")
 service:=fs.String("service","","Filter by service substring")
 limit:=fs.Int("limit",2000,"Maximum records per query")
 maxQueries:=fs.Int("max-queries",32,"Maximum API queries per update")
 bufferLines:=fs.Int("buffer-lines",20000,"Records retained for deduplication")
 output:=fs.String("output","text","text or ndjson")
 noUI:=fs.Bool("no-ui",false,"Disable full-screen interactive mode")
 group:=fs.Bool("group",false,"Group snapshot text output by service (with --once)")
 splitPanes:=fs.Bool("split-panes",false,"Open Windows Terminal panes for selected workloads")
 serviceExact:=fs.Bool("service-exact",false,"Match exact workload name (used for split pane children)")
 all:=fs.Bool("all",false,"Open all services together without the workload picker")
 noUpdateCheck:=fs.Bool("no-update-check",false,"Skip startup release check")
 if err:=fs.Parse(args);err!=nil{return 2}
 if fs.NArg()!=0{fmt.Fprintln(stderr,"unexpected positional arguments:",strings.Join(fs.Args()," "));return 2}
 ns:=*namespace
 if *nsLong!=""{
  if ns!=""{fmt.Fprintln(stderr,"use either -n or --namespace");return 2}
  ns=*nsLong
 }
 if ns==""{fmt.Fprintln(stderr,"namespace required: dynar -n apollo");return 2}
 if *since<=0||*since>30*24*time.Hour{fmt.Fprintln(stderr,"--since must be greater than zero and no more than 720h");return 2}
 if *refresh<time.Second{fmt.Fprintln(stderr,"--refresh must be at least 1s");return 2}
 if *limit<1||*limit>100000||*maxQueries<1||*maxQueries>256||*bufferLines<1{fmt.Fprintln(stderr,"invalid --limit, --max-queries, or --buffer-lines");return 2}
 if *output!="text"&&*output!="ndjson"{fmt.Fprintln(stderr,"--output must be text or ndjson");return 2}
 if *group && (!(*once||*noFollow) || *output!="text"){fmt.Fprintln(stderr,"--group requires --once and --output text");return 2}
 if *splitPanes && (*once||*noFollow||*noUI||*output!="text"){fmt.Fprintln(stderr,"--split-panes requires interactive live text mode");return 2}
 if len(ns)>253||strings.ContainsAny(ns,"\r\n\x00"){fmt.Fprintln(stderr,"invalid namespace");return 2}
 matcher,err:=makeMatcher(*search,*regex,*level,*service)
 if err!=nil{fmt.Fprintln(stderr,err);return 2}
 cfg,err:=config.Load()
 if err!=nil{fmt.Fprintln(stderr,err);return 1}
 name:=*source
 if name==""{name=cfg.Default}
 var base string
 if name!=""{
  src,exists:=cfg.Sources[name]
  if !exists{fmt.Fprintf(stderr,"unknown source %q; configure with: dynar source add %s --url https://TENANT.apps.dynatrace.com\n",name,name);return 2}
  base=src.URL
 }else{
  name="default"
  base=config.Env("","URL")
 }
 if base==""{fmt.Fprintln(stderr,"no Dynatrace source configured. Run: dynar source add prod --url https://TENANT.apps.dynatrace.com --default");return 2}
 client,err:=dynatrace.NewClient(base,dynatrace.Credentials{Token:config.Env(name,"TOKEN"),ClientID:config.Env(name,"CLIENT_ID"),ClientSecret:config.Env(name,"CLIENT_SECRET")})
 if err!=nil{fmt.Fprintln(stderr,err);return 2}
 collector:=Collector{Query:client.Query,Limit:*limit,MaxQueries:*maxQueries}
 follow:=!*once&&!*noFollow
 if follow && !*noUI && *output=="text" && interactiveTerminal(stdout) {
  restart,err:=sharedViewer(ctx,collector,SharedOptions{
   Source:name,Namespace:ns,Cluster:*cluster,Service:*service,
   Since:*since,Refresh:*refresh,BufferLines:*bufferLines,
   NoUpdateCheck:*noUpdateCheck,All:*all,Search:*search,Regex:*regex,
   SplitPanes:*splitPanes,ServiceExact:*serviceExact,Level:*level,
  },stdout,stderr)
  if err!=nil{fmt.Fprintln(stderr,"DynaR:",err);return 1}
  if restart{return RestartExitCode}
  return 0
 }
 store:=logs.NewStore(*bufferLines)
 lastEnd:=time.Now().UTC()
 windowStart:=lastEnd.Add(-*since)
 fmt.Fprintf(stderr,"DynaR | source=%s namespace=%s since=%s follow=%t\n",name,ns,since.String(),follow)
 for{
  events,warnings,err:=collector.Load(ctx,name,ns,*cluster,windowStart,lastEnd)
  if err!=nil{fmt.Fprintln(stderr,"query failed:",err);return 1}
  newEvents:=store.Add(events)
  if *group{
   byService:=map[string][]logs.Event{}
   for _,e:=range newEvents{if matcher(e){byService[e.Service]=append(byService[e.Service],e)}}
   services:=make([]string,0,len(byService))
   for service:=range byService{services=append(services,service)}
   sort.Strings(services)
   for _,service:=range services{
    fmt.Fprintf(stdout,"\n=== %s (%d records) ===\n",logs.SafeText(service),len(byService[service]))
    for _,event:=range byService[service]{fmt.Fprintln(stdout,format(event))}
   }
  }
  for _,event:=range newEvents{
   if *group||!matcher(event){continue}
   if *output=="ndjson"{
    raw,err:=json.Marshal(event)
    if err!=nil{fmt.Fprintln(stderr,err);return 1}
    fmt.Fprintln(stdout,string(raw))
   }else{fmt.Fprintln(stdout,format(event))}
  }
  for _,warning:=range warnings{fmt.Fprintln(stderr,"WARNING:",warning)}
  if !follow{
   if len(newEvents)==0{fmt.Fprintln(stderr,"No logs returned in the selected time range")}
   return 0
  }
  timer:=time.NewTimer(*refresh)
  select {case <-ctx.Done():timer.Stop();return 0;case <-timer.C:}
  // Overlap allows for late ingestion while bounded in-memory deduplication
  // suppresses records already displayed.
  windowStart=lastEnd.Add(-5*time.Minute)
  lastEnd=time.Now().UTC()
 }
}
func format(e logs.Event)string{
 header:=fmt.Sprintf("%s [%s] [%s] ",e.Timestamp.Local().Format("2006-01-02 15:04:05.000"),logs.LevelCode(e.Level),logs.SafeText(e.Service))
 return header+strings.Join(logs.PrettyMessage(e),"\n    ")
}
func interactiveTerminal(w io.Writer)bool{
 out,ok:=w.(*os.File)
 if !ok{return false}
 outInfo,err:=out.Stat()
 if err!=nil || outInfo.Mode()&os.ModeCharDevice==0{return false}
 inInfo,err:=os.Stdin.Stat()
 return err==nil && inInfo.Mode()&os.ModeCharDevice!=0
}

func makeMatcher(term string,useRegex bool,level,service string)(func(logs.Event)bool,error){
 var expression *regexp.Regexp
 if useRegex&&term!=""{
  var err error
  expression,err=regexp.Compile("(?i)"+term)
  if err!=nil{return nil,fmt.Errorf("invalid regex: %w",err)}
 }
 return func(e logs.Event)bool{
  if level!=""&&!strings.EqualFold(e.Level,level){return false}
  if service!=""&&!strings.Contains(strings.ToLower(e.Service),strings.ToLower(service)){return false}
  if term==""{return true}
  value:=strings.Join([]string{e.Message,e.Service,e.Pod,e.Container,e.TraceID,e.RequestID,e.Level,e.Namespace,e.Source}," ")
  if expression!=nil{return expression.MatchString(value)}
  return strings.Contains(strings.ToLower(value),strings.ToLower(term))
 },nil
}
func sources(args []string,stdout,stderr io.Writer)int{
 cfg,err:=config.Load();if err!=nil{fmt.Fprintln(stderr,err);return 1}
 if len(args)==0{fmt.Fprintln(stderr,"usage: dynar source add <name> --url <url> [--default] | source list | source use <name>");return 2}
 switch args[0]{
 case "list":
  names:=make([]string,0,len(cfg.Sources))
  for k:=range cfg.Sources{names=append(names,k)}
  sort.Strings(names)
  for _,name:=range names{
   mark:=" ";if name==cfg.Default{mark="*"}
   fmt.Fprintf(stdout,"%s %-18s %s  token-env=%sTOKEN\n",mark,name,cfg.Sources[name].URL,config.TokenPrefix(name))
  }
  if len(names)==0{fmt.Fprintln(stdout,"No sources configured")}
  return 0
 case "add":
  if len(args)<2{fmt.Fprintln(stderr,"usage: dynar source add <name> --url https://TENANT.apps.dynatrace.com [--default]");return 2}
  name:=args[1]
  if err:=config.ValidateName(name);err!=nil{fmt.Fprintln(stderr,err);return 2}
  fs:=flag.NewFlagSet("source add",flag.ContinueOnError);fs.SetOutput(stderr)
  base:=fs.String("url","","Dynatrace environment base URL")
  setDefault:=fs.Bool("default",false,"Make this the default source")
  if err:=fs.Parse(args[2:]);err!=nil{return 2}
  if fs.NArg()!=0||*base==""{fmt.Fprintln(stderr,"--url is required");return 2}
  if err:=dynatrace.ValidateURL(*base);err!=nil{fmt.Fprintln(stderr,err);return 2}
  cfg.Sources[name]=config.Source{URL:strings.TrimRight(*base,"/")}
  if *setDefault||cfg.Default==""{cfg.Default=name}
  if err:=cfg.Save();err!=nil{fmt.Fprintln(stderr,err);return 1}
  fmt.Fprintf(stdout,"Saved source %q. Set %sTOKEN (platform bearer token) or %sCLIENT_ID / %sCLIENT_SECRET.\n",name,config.TokenPrefix(name),config.TokenPrefix(name),config.TokenPrefix(name))
  return 0
 case "use":
  if len(args)!=2{fmt.Fprintln(stderr,"usage: dynar source use <name>");return 2}
  if _,exists:=cfg.Sources[args[1]];!exists{fmt.Fprintln(stderr,"unknown source",args[1]);return 2}
  cfg.Default=args[1]
  if err:=cfg.Save();err!=nil{fmt.Fprintln(stderr,err);return 1}
  fmt.Fprintln(stdout,"Default source:",cfg.Default);return 0
 default:fmt.Fprintln(stderr,"unknown source subcommand",args[0]);return 2
 }
}
func usage(w io.Writer){
 fmt.Fprint(w,`DynaR - Dynatrace namespace-wide log explorer

  dynar source add prod --url https://TENANT.apps.dynatrace.com --default
  dynar -n apollo
  dynar -n apollo --once --since 24h
  dynar -n apollo --source prod --search timeout --level ERROR
  dynar -n apollo --source qa --cluster tkgs-qa --output ndjson
  dynar -n apollo --once --group
  dynar -n apollo --all
  dynar -n apollo --split-panes
  dynar update --check
  dynar update

Credentials: DYNAR_TOKEN, or DYNAR_<SOURCE>_TOKEN (e.g. DYNAR_PROD_TOKEN).
OAuth client credentials: DYNAR_CLIENT_ID and DYNAR_CLIENT_SECRET (or source-prefixed).
Tokens are read from environment variables, never saved in source configuration.

Options: -n, --namespace, --source, --cluster, --since, --refresh,
         --once, --no-follow, --search, --regex, --level, --service,
         --output text|ndjson, --no-ui, --group (snapshots), --all,
         --split-panes, --no-update-check,
         --limit, --max-queries, --buffer-lines.
`)
}
