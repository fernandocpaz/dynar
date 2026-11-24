package credentials

import (
 "errors"
 "testing"
)

type fakeStore struct {
 data map[string]string
 fail bool
}
func (f *fakeStore) key(name,field string)string{return name+"/"+field}
func (f *fakeStore) Get(name,field string)(string,error){
 if f.fail{return "",errors.New("keyring locked")}
 return f.data[f.key(name,field)],nil
}
func (f *fakeStore) Set(name,field,value string)error{
 if f.fail{return errors.New("keyring locked")}
 f.data[f.key(name,field)]=value
 return nil
}
func (f *fakeStore) Delete(name,field string)error{
 if f.fail{return errors.New("keyring locked")}
 delete(f.data,f.key(name,field))
 return nil
}
func TestVaultRoundtripAndMethodSwitch(t *testing.T){
 store:=&fakeStore{data:map[string]string{}}
 if err:=Replace("prod",store,Auth{Token:"private-token"});err!=nil{t.Fatal(err)}
 auth:=Resolve("prod",store)
 if auth.Token!="private-token"||auth.Method()!="token"{t.Fatalf("missing token: %+v",auth)}
 if err:=Replace("prod",store,Auth{ClientID:"client",ClientSecret:"password"});err!=nil{t.Fatal(err)}
 auth=Resolve("prod",store)
 if auth.Token!=""||auth.Method()!="oauth"||auth.ClientID!="client"{t.Fatalf("method switch failed: %+v",auth)}
 if err:=Clear("prod",store);err!=nil{t.Fatal(err)}
 if Resolve("prod",store).Ready(){t.Fatal("cleared vault returned credentials")}
}
func TestEnvironmentOverridesVaultWithoutChangingIt(t *testing.T){
 store:=&fakeStore{data:map[string]string{"qa/token":"stored-token"}}
 t.Setenv("DYNAR_QA_TOKEN","environment-token")
 if got:=Resolve("qa",store);got.Token!="environment-token"{t.Fatal("environment override failed")}
 if got:=CredentialSource("qa",store);got!="token from environment"{t.Fatalf("status: %s",got)}
 if store.data["qa/token"]!="stored-token"{t.Fatal("vault contents changed")}
}
func TestUnavailableVaultDoesNotReturnAnAuth(t *testing.T){
 store:=&fakeStore{data:map[string]string{},fail:true}
 if Resolve("prod",store).Ready(){t.Fatal("vault failure must not be treated as valid credentials")}
 if err:=Replace("prod",store,Auth{Token:"x"});err==nil{t.Fatal("vault write failure ignored")}
}
