package app

import (
 "bufio"
 "errors"
 "fmt"
 "io"
 "os"
 "sort"
 "strings"
 "golang.org/x/term"
 "github.com/fernandocpaz/dynar/internal/config"
 "github.com/fernandocpaz/dynar/internal/credentials"
 "github.com/fernandocpaz/dynar/internal/dynatrace"
)
var errSetupCancelled=errors.New("setup cancelled")
func interactiveSetup(out io.Writer)bool{
 f,ok:=out.(*os.File)
 return ok&&term.IsTerminal(int(os.Stdin.Fd()))&&term.IsTerminal(int(f.Fd()))
}
type wizard struct{
 reader *bufio.Reader
 out io.Writer
 secret func(string)(string,error)
 vault credentials.Store
}
func newWizard(out io.Writer)*wizard{
 return &wizard{
  reader:bufio.NewReader(os.Stdin),out:out,vault:credentials.NativeStore{},
  secret:func(label string)(string,error){
   fmt.Fprint(out,label,": ")
   value,err:=term.ReadPassword(int(os.Stdin.Fd()))
   fmt.Fprintln(out)
   return strings.TrimSpace(string(value)),err
  },
 }
}
func(w *wizard) ask(label,existing string,validate func(string)error)(string,error){
 for {
  if existing!=""{fmt.Fprintf(w.out,"%s [%s]: ",label,existing)}else{fmt.Fprintf(w.out,"%s: ",label)}
  value,err:=w.reader.ReadString('\n')
  if err!=nil&&!(errors.Is(err,io.EOF)&&len(value)>0){return "",errSetupCancelled}
  value=strings.TrimSpace(value)
  if value==""{value=existing}
  if validate==nil{return value,nil}
  if err:=validate(value);err!=nil{fmt.Fprintln(w.out,"  "+err.Error());continue}
  return value,nil
 }
}
func(w *wizard) choose(label,existing string,options ...string)(string,error){
 return w.ask(label,existing,func(value string)error{
  for _,option:=range options{if strings.EqualFold(value,option){return nil}}
  return fmt.Errorf("choose %s",strings.Join(options,", "))
 })
}
func savedSources(c config.Config)[]string{
 names:=make([]string,0,len(c.Sources))
 for name:=range c.Sources{names=append(names,name)}
 sort.Strings(names)
 return names
}
func(w *wizard) authEntry(mode string)(credentials.Auth,error){
 switch strings.ToLower(mode){
 case "token":
  for{
   value,err:=w.secret("Dynatrace platform token (hidden)")
   if err!=nil{return credentials.Auth{},err}
   if value!=""{return credentials.Auth{Token:value},nil}
   fmt.Fprintln(w.out,"A token is required.")
  }
 case "oauth":
  id,err:=w.ask("OAuth client ID","",func(v string)error{
   if v==""{return errors.New("client ID is required")}
   return nil
  })
  if err!=nil{return credentials.Auth{},err}
  for{
   value,err:=w.secret("OAuth client secret (hidden)")
   if err!=nil{return credentials.Auth{},err}
   if value!=""{return credentials.Auth{ClientID:id,ClientSecret:value},nil}
   fmt.Fprintln(w.out,"A client secret is required.")
  }
 }
 return credentials.Auth{},errors.New("unsupported credential type")
}
// configure writes only public connection details to config.json. Secrets
// are stored through the OS vault. On vault failure, they remain session-only.
func(w *wizard) configure(cfg config.Config,requested string,edit bool)(config.Config,string,credentials.Auth,error){
 if cfg.Sources==nil{cfg.Sources=make(map[string]config.Source)}
 fmt.Fprintln(w.out,"\nDynaR setup (Enter accepts [defaults]; Ctrl+C cancels)")
 if names:=savedSources(cfg);len(names)>0{fmt.Fprintln(w.out,"Available sources:",strings.Join(names,", "))}
 name:=requested
 if name==""{name=cfg.Default}
 if name==""{if names:=savedSources(cfg);len(names)>0{name=names[0]}else{name="prod"}}
 if edit||cfg.Default==""||config.ValidateName(name)!=nil{
  var err error
  name,err=w.ask("Default Dynatrace source",name,config.ValidateName)
  if err!=nil{return cfg,"",credentials.Auth{},err}
  cfg.Default=name
 }
 if err:=config.ValidateName(name);err!=nil{return cfg,"",credentials.Auth{},err}
 src:=cfg.Sources[name]
 url:=src.URL
 if url==""{url=config.Env(name,"URL")}
 url,err:=w.ask("Dynatrace URL",url,dynatrace.ValidateURL)
 if err!=nil{return cfg,"",credentials.Auth{},err}
 src.URL=strings.TrimRight(url,"/")
 cfg.Sources[name]=src
 if edit||cfg.Namespace==""{
  cfg.Namespace,err=w.ask("Default namespace",cfg.EffectiveNamespace(),func(v string)error{
   if v==""||len(v)>253||strings.ContainsAny(v," \r\n\x00"){return errors.New("invalid namespace")}
   return nil
  })
  if err!=nil{return cfg,"",credentials.Auth{},err}
 }
 if edit||cfg.Cluster==""{
  cfg.Cluster,err=w.ask("Default cluster",cfg.EffectiveCluster(),func(v string)error{
   if v==""||strings.ContainsAny(v," \r\n\x00"){return errors.New("invalid cluster")}
   return nil
  })
  if err!=nil{return cfg,"",credentials.Auth{},err}
 }
 if edit||cfg.Monitor==nil{
  current:="yes";if !cfg.EffectiveMonitor(){current="no"}
  answer,err:=w.choose("Monitor by default? (yes/no)",current,"yes","no")
  if err!=nil{return cfg,"",credentials.Auth{},err}
  enabled:=strings.EqualFold(answer,"yes")
  cfg.Monitor=&enabled
 }
 current:=credentials.Resolve(name,w.vault)
 var entered credentials.Auth
 if current.Ready(){
  if edit{
   fmt.Fprintln(w.out,"Credentials:",credentials.CredentialSource(name,w.vault),"(secret values hidden)")
   action,err:=w.choose("Credentials (keep/token/oauth/clear)","keep","keep","token","oauth","clear")
   if err!=nil{return cfg,"",credentials.Auth{},err}
   switch strings.ToLower(action){
   case "clear":
    if err:=credentials.Clear(name,w.vault);err!=nil{fmt.Fprintln(w.out,"Warning: unable to clear OS credential vault; environment credentials are never changed.")}
    current=credentials.Resolve(name,w.vault)
   case "keep":
   default:
    entered,err=w.authEntry(action)
    if err!=nil{return cfg,"",credentials.Auth{},err}
   }
  }
 }else{
  fmt.Fprintln(w.out,"No credentials detected: enter a token or OAuth client credentials.")
  choice,err:=w.choose("Authentication (token/oauth)","token","token","oauth")
  if err!=nil{return cfg,"",credentials.Auth{},err}
  entered,err=w.authEntry(choice)
  if err!=nil{return cfg,"",credentials.Auth{},err}
 }
 if err:=cfg.Save();err!=nil{return cfg,"",credentials.Auth{},err}
 if entered.Ready(){
  if err:=credentials.Replace(name,w.vault,entered);err!=nil{
   fmt.Fprintln(w.out,"Warning: OS credential vault unavailable. Credentials are valid for this run only.")
   fmt.Fprintln(w.out,"For future runs, set",config.TokenPrefix(name)+"TOKEN","or the source's CLIENT_ID and CLIENT_SECRET environment variables.")
  }else{fmt.Fprintln(w.out,"Credentials saved securely in the OS credential vault.")}
  current=entered
 }
 fmt.Fprintln(w.out,"Settings saved. Use 'dynar config edit' to change them.")
 return cfg,name,current,nil
}
func configCommand(args []string,stdout,stderr io.Writer)int{
 cfg,err:=config.Load()
 if err!=nil{fmt.Fprintln(stderr,err);return 1}
 mode:="edit"
 if len(args)>0{mode=args[0]}
 if len(args)>1 || mode!="edit"&&mode!="show"{
  fmt.Fprintln(stderr,"Usage: dynar config [show|edit]")
  return 2
 }
 if mode=="show"{
  path,_:=config.Path()
  fmt.Fprintf(stdout,"Config: %s\nDefault source: %s\nNamespace: %s\nCluster: %s\nMonitor: %t\n",
   path,cfg.Default,cfg.EffectiveNamespace(),cfg.EffectiveCluster(),cfg.EffectiveMonitor())
  for _,name:=range savedSources(cfg){
   mark:=" ";if name==cfg.Default{mark="*"}
   fmt.Fprintf(stdout,"%s %s  %s  credentials: %s\n",mark,name,cfg.Sources[name].URL,credentials.CredentialSource(name,credentials.NativeStore{}))
  }
  fmt.Fprintln(stdout,"Run: dynar config edit (secret values are never displayed).")
  return 0
 }
 if !interactiveSetup(stdout)||!interactiveSetup(stderr){fmt.Fprintln(stderr,"Editor requires an interactive terminal: dynar config edit");return 2}
 w:=newWizard(stderr)
 if _,_,_,err:=w.configure(cfg,"",true);err!=nil{
  fmt.Fprintln(stderr,"Setup not completed:",err);return 1
 }
 return 0
}
// requireConfiguration is called only after parsing requested CLI options.
// Automated/non-TTY runs never prompt.
func requireConfiguration(cfg config.Config,requested string,console bool,stderr io.Writer)(config.Config,string,credentials.Auth,error){
 name:=requested
 if name==""{name=cfg.Default}
 if name==""&&config.Env("","URL")!=""{name="default"}
 url:=""
 if s,ok:=cfg.Sources[name];ok{url=s.URL}
 if url==""{url=config.Env(name,"URL")}
 auth:=credentials.Resolve(name,credentials.NativeStore{})
 invalidURL:=url!=""&&dynatrace.ValidateURL(url)!=nil
 missingPreferences:=cfg.Namespace==""||cfg.Cluster==""||cfg.Monitor==nil
 missing:=name==""||url==""||invalidURL||!auth.Ready()||console&&requested==""&&(cfg.Default==""||missingPreferences)
 if !missing{return cfg,name,auth,nil}
 if !console {
  return cfg,"",credentials.Auth{},errors.New("Dynatrace source, URL or credentials missing. Run 'dynar setup' in a terminal, or configure DYNAR_URL and DYNAR_TOKEN (or OAuth variables)")
 }
 w:=newWizard(stderr)
 updated,name,session,err:=w.configure(cfg,requested,false)
 if err!=nil{return cfg,"",credentials.Auth{},err}
 if !session.Ready(){session=credentials.Resolve(name,w.vault)}
 if !session.Ready(){return cfg,"",credentials.Auth{},errors.New("no usable Dynatrace credentials")}
 return updated,name,session,nil
}
