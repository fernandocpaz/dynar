package config

import (
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "regexp"
 "strings"
)

type Source struct { URL string `json:"url"` }
type Config struct {
 Default string `json:"default"`
 Sources map[string]Source `json:"sources"`
}
var validName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,48}$`)

func Path() (string,error) {
 if p := os.Getenv("DYNAR_CONFIG"); p!="" { return p,nil }
 dir,err:=os.UserConfigDir()
 if err!=nil{return "",err}
 return filepath.Join(dir,"dynar","config.json"),nil
}
func Load() (Config,error) {
 path,err:=Path();if err!=nil{return Config{},err}
 c:=Config{Sources:map[string]Source{}}
 b,err:=os.ReadFile(path)
 if errors.Is(err,os.ErrNotExist){return c,nil}
 if err!=nil{return c,err}
 if err=json.Unmarshal(b,&c);err!=nil{return c,fmt.Errorf("invalid config %s: %w",path,err)}
 if c.Sources==nil{c.Sources=map[string]Source{}}
 return c,nil
}
func (c Config) Save() error {
 path,err:=Path();if err!=nil{return err}
 dir:=filepath.Dir(path)
 if err=os.MkdirAll(dir,0700);err!=nil{return err}
 b,err:=json.MarshalIndent(c,"","  ");if err!=nil{return err}
 // Configuration contains URLs and names only, never secret tokens.
 tmp,err:=os.CreateTemp(dir,".config-*");if err!=nil{return err}
 defer os.Remove(tmp.Name())
 if err=tmp.Chmod(0600);err!=nil{tmp.Close();return err}
 if _,err=tmp.Write(append(b,'\n'));err!=nil{tmp.Close();return err}
 if err=tmp.Close();err!=nil{return err}
 return os.Rename(tmp.Name(),path)
}
func ValidateName(name string)error{
 if !validName.MatchString(name){return fmt.Errorf("invalid source %q: use letters, digits, _ or -; start with a letter",name)}
 return nil
}
func TokenPrefix(name string)string{
 return "DYNAR_"+strings.ToUpper(strings.NewReplacer("-","_",".","_").Replace(name))+"_"
}
func Env(name,suffix string)string{
 if name!=""{if v:=os.Getenv(TokenPrefix(name)+suffix);v!=""{return v}}
 return os.Getenv("DYNAR_"+suffix)
}
