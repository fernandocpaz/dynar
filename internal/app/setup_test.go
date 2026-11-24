package app

import (
 "bufio"
 "bytes"
 "errors"
 "io"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/fernandocpaz/dynar/internal/config"
 "github.com/fernandocpaz/dynar/internal/credentials"
)
type wizardVault struct{
 values map[string]string
 writeFailure bool
}
func(v *wizardVault) Get(name,field string)(string,error){return v.values[name+"/"+field],nil}
func(v *wizardVault) Set(name,field,value string)error{
 if v.writeFailure{return errors.New("keyring unavailable")}
 v.values[name+"/"+field]=value;return nil
}
func(v *wizardVault) Delete(name,field string)error{delete(v.values,name+"/"+field);return nil}
func testWizard(input string,v *wizardVault,out *bytes.Buffer)*wizard{
 return &wizard{reader:bufio.NewReader(strings.NewReader(input)),out:out,vault:v,
  secret:func(string)(string,error){return "VERY-SECRET-TOKEN",nil}}
}
func TestFirstRunWizardPersistsSourceDefaultsAndOSVaultOnly(t *testing.T){
 path:=filepath.Join(t.TempDir(),"dynar","config.json")
 t.Setenv("DYNAR_CONFIG",path)
 out:=new(bytes.Buffer)
 v:=&wizardVault{values:map[string]string{}}
 // default source, URL, namespace, cluster, monitor, token auth method
 w:=testWizard("\nhttps://test.apps.dynatrace.com\n\n\n\n\n",v,out)
 cfg,name,auth,err:=w.configure(config.Config{},"",false)
 if err!=nil{t.Fatal(err)}
 if name!="prod"||cfg.Default!="prod"||cfg.Sources[name].URL!="https://test.apps.dynatrace.com"{
  t.Fatalf("source config wrong: %+v",cfg)
 }
 if cfg.EffectiveNamespace()!="apollo"||cfg.EffectiveCluster()!="tkgs-prod"||!cfg.EffectiveMonitor(){
  t.Fatalf("default preferences wrong: %+v",cfg)
 }
 if auth.Token!="VERY-SECRET-TOKEN"||v.values["prod/token"]!="VERY-SECRET-TOKEN"{
  t.Fatal("masked credential was not saved to vault")
 }
 raw,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
 if bytes.Contains(raw,[]byte("VERY-SECRET-TOKEN"))||strings.Contains(out.String(),"VERY-SECRET-TOKEN"){
  t.Fatal("secret leaked into JSON configuration or terminal output")
 }
 loaded,err:=config.Load();if err!=nil{t.Fatal(err)}
 if loaded.Default!="prod"||loaded.Namespace!="apollo"||loaded.Cluster!="tkgs-prod"{
  t.Fatalf("saved profile not reloaded: %+v",loaded)
 }
}
func TestEditChangesURLDefaultsAndReplacesAuth(t *testing.T){
 t.Setenv("DYNAR_CONFIG",filepath.Join(t.TempDir(),"config.json"))
 enabled:=true
 cfg:=config.Config{Default:"prod",Sources:map[string]config.Source{"prod":{URL:"https://old.apps.dynatrace.com"}},Namespace:"apollo",Cluster:"tkgs-prod",Monitor:&enabled}
 v:=&wizardVault{values:map[string]string{"prod/token":"oldtoken"}}
 out:=new(bytes.Buffer)
 w:=testWizard("\nhttps://new.apps.dynatrace.com\nemrtrain\ntkgs-qa\nno\noauth\nclient-123\n",v,out)
 changed,_,auth,err:=w.configure(cfg,"",true)
 if err!=nil{t.Fatal(err)}
 if changed.Sources["prod"].URL!="https://new.apps.dynatrace.com"||changed.Namespace!="emrtrain"||
  changed.Cluster!="tkgs-qa"||changed.EffectiveMonitor(){t.Fatalf("edits missing: %+v",changed)}
 if auth.Method()!="oauth"||auth.ClientID!="client-123"||auth.ClientSecret!="VERY-SECRET-TOKEN"{
  t.Fatalf("oauth auth missing (method %s)",auth.Method())
 }
 if v.values["prod/token"]!=""||v.values["prod/client-secret"]!="VERY-SECRET-TOKEN"{
  t.Fatal("old token not purged or secret not written")
 }
}
func TestVaultFailureUsesSessionCredentialsWithoutPlaintextPersistence(t *testing.T){
 t.Setenv("DYNAR_CONFIG",filepath.Join(t.TempDir(),"config.json"))
 v:=&wizardVault{values:map[string]string{},writeFailure:true}
 var out bytes.Buffer
 w:=testWizard("\nhttps://test.apps.dynatrace.com\n\n\n\n\n",v,&out)
 _,_,auth,err:=w.configure(config.Config{},"",false)
 if err!=nil{t.Fatal(err)}
 if !auth.Ready(){t.Fatal("session auth lost after vault failed")}
 if !strings.Contains(out.String(),"this run only"){t.Fatal("must warn credentials not persistent")}
 saved,err:=os.ReadFile(os.Getenv("DYNAR_CONFIG"));if err!=nil{t.Fatal(err)}
 if bytes.Contains(saved,[]byte(auth.Token)){t.Fatal("plaintext secret saved to JSON")}
}
func TestNoninteractiveSetupIsActionableAndNeverPrompts(t *testing.T){
 _,_,_,err:=requireConfiguration(config.Config{},"",false,io.Discard)
 if err==nil||!strings.Contains(err.Error(),"dynar setup"){t.Fatalf("expected setup instruction, got %v",err)}
}
func TestConfigShowDoesNotExposeTokenFromEnvironment(t *testing.T){
 t.Setenv("DYNAR_CONFIG",filepath.Join(t.TempDir(),"config.json"))
 t.Setenv("DYNAR_PROD_TOKEN","PRIVATE-TOKEN")
 cfg:=config.Config{Default:"prod",Sources:map[string]config.Source{"prod":{URL:"https://test.apps.dynatrace.com"}}}
 if err:=cfg.Save();err!=nil{t.Fatal(err)}
 var out,stderr bytes.Buffer
 if code:=configCommand([]string{"show"},&out,&stderr);code!=0{t.Fatalf("show failed %d: %s",code,stderr.String())}
 if strings.Contains(out.String(),"PRIVATE-TOKEN")||!strings.Contains(out.String(),"token from environment"){
  t.Fatal("config show leaked secret or hid source")
 }
}
