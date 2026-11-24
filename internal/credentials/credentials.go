package credentials

import (
 "errors"
 "os"
 "strings"

 "github.com/zalando/go-keyring"
)

const Service = "DynaR"
const (
 Token = "token"
 ClientID = "client-id"
 ClientSecret = "client-secret"
)

// Store exposes native Windows Credential Manager, macOS Keychain and Linux
// Secret Service. It never saves secrets into DynaR's JSON configuration.
type Store interface {
 Get(source, field string) (string,error)
 Set(source, field, value string) error
 Delete(source, field string) error
}
type NativeStore struct{}
func account(source,field string)string{return "source/"+strings.ToLower(source)+"/"+field}
func (NativeStore) Get(source,field string)(string,error){
 return keyring.Get(Service,account(source,field))
}
func (NativeStore) Set(source,field,value string)error{
 return keyring.Set(Service,account(source,field),value)
}
func (NativeStore) Delete(source,field string)error{
 err:=keyring.Delete(Service,account(source,field))
 if errors.Is(err,keyring.ErrNotFound){return nil}
 return err
}
type Auth struct {
 Token string
 ClientID string
 ClientSecret string
}
func (a Auth) Ready()bool{return a.Token!="" || a.ClientID!=""&&a.ClientSecret!=""}
func (a Auth) Method()string{
 if a.Token!=""{return "token"}
 if a.ClientID!=""&&a.ClientSecret!=""{return "oauth"}
 return "missing"
}
func env(source,field string)string{
 suffix:=strings.ToUpper(strings.ReplaceAll(field,"-","_"))
 source=strings.ToUpper(strings.NewReplacer("-","_",".","_").Replace(source))
 if source!=""{
  if s:=os.Getenv("DYNAR_"+source+"_"+suffix);s!=""{return s}
 }
 return os.Getenv("DYNAR_"+suffix)
}
// Resolve uses environment variables first, then the OS vault. An unavailable
// vault is not treated as a fatal error if environment credentials exist.
func Resolve(source string,store Store)Auth{
 get:=func(field string)string{
  if value:=env(source,field);value!=""{return value}
  if store==nil{return ""}
  value,err:=store.Get(source,field)
  if err!=nil{return ""}
  return value
 }
 return Auth{Token:get(Token),ClientID:get(ClientID),ClientSecret:get(ClientSecret)}
}
func CredentialSource(source string,store Store)string{
 if env(source,Token)!=""{return "token from environment"}
 if env(source,ClientID)!=""&&env(source,ClientSecret)!=""{return "OAuth from environment"}
 if store!=nil{
  if token,err:=store.Get(source,Token);err==nil&&token!=""{return "token from OS credential vault"}
  id,errID:=store.Get(source,ClientID)
  secret,errSecret:=store.Get(source,ClientSecret)
  if errID==nil&&errSecret==nil&&id!=""&&secret!=""{return "OAuth from OS credential vault"}
 }
 return "not configured"
}
// Replace stores a coherent single authentication method. Existing vault
// credentials are removed only AFTER new values were stored successfully.
func Replace(source string,store Store,auth Auth)error{
 if store==nil{return errors.New("system credential vault is unavailable")}
 switch auth.Method(){
 case "token":
  if err:=store.Set(source,Token,auth.Token);err!=nil{return err}
  _=store.Delete(source,ClientID)
  _=store.Delete(source,ClientSecret)
 case "oauth":
  if err:=store.Set(source,ClientID,auth.ClientID);err!=nil{return err}
  if err:=store.Set(source,ClientSecret,auth.ClientSecret);err!=nil{return err}
  _=store.Delete(source,Token)
 default:return errors.New("no complete credentials supplied")
 }
 return nil
}
func Clear(source string,store Store)error{
 if store==nil{return errors.New("system credential vault is unavailable")}
 for _,field:=range []string{Token,ClientID,ClientSecret}{
  if err:=store.Delete(source,field);err!=nil{return err}
 }
 return nil
}
