package config
import (
 "os"
 "path/filepath"
 "testing"
)
func TestSavedPreferencesAreOptionalAndBackwardCompatible(t *testing.T){
 t.Setenv("DYNAR_CONFIG",filepath.Join(t.TempDir(),"config.json"))
 old:=[]byte(`{"default":"prod","sources":{"prod":{"url":"https://example.apps.dynatrace.com"}}}`)
 if err:=os.WriteFile(os.Getenv("DYNAR_CONFIG"),old,0600);err!=nil{t.Fatal(err)}
 loaded,err:=Load();if err!=nil{t.Fatal(err)}
 if loaded.EffectiveNamespace()!="apollo"||loaded.EffectiveCluster()!="tkgs-prod"||!loaded.EffectiveMonitor(){
  t.Fatalf("existing config defaults changed: %+v",loaded)
 }
 no:=false
 loaded.Monitor=&no
 loaded.Namespace="emrtrain";loaded.Cluster="tkgs-qa"
 if err:=loaded.Save();err!=nil{t.Fatal(err)}
 again,err:=Load();if err!=nil{t.Fatal(err)}
 if again.EffectiveMonitor()||again.EffectiveNamespace()!="emrtrain"||again.EffectiveCluster()!="tkgs-qa"{
  t.Fatalf("edited preferences not preserved: %+v",again)
 }
}
