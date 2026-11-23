package dynatrace

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)
func TestQueryExecuteThenPoll(t *testing.T){
 calls:=0
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  calls++
  if r.Header.Get("Authorization")!="Bearer demo"{t.Errorf("missing auth");w.WriteHeader(401);return}
  switch r.URL.Path {
  case "/platform/storage/query/v1/query:execute":
   if r.Method!="POST"{t.Errorf("wrong method")}
   var m map[string]any
   if err:=json.NewDecoder(r.Body).Decode(&m);err!=nil{t.Error(err)}
   if !strings.Contains(m["query"].(string),`k8s.namespace.name == "apollo"`){t.Errorf("unexpected dql: %s",m["query"])}
   w.WriteHeader(http.StatusAccepted)
   _,_=w.Write([]byte(`{"state":"RUNNING","requestToken":"abc"}`))
  case "/platform/storage/query/v1/query:poll":
   if r.URL.Query().Get("request-token")!="abc"{t.Errorf("missing request token")}
   _,_=w.Write([]byte(`{"state":"SUCCEEDED","result":{"records":[{"timestamp":"2026-11-23T15:00:00Z","content":"hello"}],"metadata":{"grail":{"sampled":false}}}}`))
  default:
   t.Errorf("unknown path %s",r.URL.Path);w.WriteHeader(404)
  }
 }))
 defer srv.Close()
 c,err:=NewClient(srv.URL,Credentials{Token:"demo"});if err!=nil{t.Fatal(err)}
 c.HTTP=srv.Client()
 start:=time.Date(2026,11,23,14,0,0,0,time.UTC)
 result,err:=c.Query(context.Background(),"apollo","",start,start.Add(2*time.Hour),10)
 if err!=nil{t.Fatal(err)}
 if len(result.Records)!=1||calls!=2{t.Fatalf("got %d records after %d calls",len(result.Records),calls)}
}
func TestValidateURL(t *testing.T){
 for _,value:=range []string{"http://example.com","https://user:password@example.com","https://example.com/e/123","https://example.com?token=bad","file:///tmp/test"}{
  if err:=ValidateURL(value);err==nil{t.Errorf("accepted unsafe URL %q",value)}
 }
 if err:=ValidateURL("https://example.apps.dynatrace.com");err!=nil{t.Fatal(err)}
}
func TestDQLQuotedNamespace(t *testing.T){
 q:=DQL("evil\\\" | limit 1","qa",100)
 if !strings.Contains(q,`"evil\\\" | limit 1"`){t.Fatalf("DQL not escaped: %s",q)}
}
