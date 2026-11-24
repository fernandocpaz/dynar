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
func TestMetricsOAuthUsesDedicatedScopes(t *testing.T){
 var scope string
 mock:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if err:=r.ParseForm();err!=nil{t.Error(err)}
  scope=r.Form.Get("scope")
  _,_=w.Write([]byte(`{"access_token":"abc","expires_in":3600}`))
 }))
 defer mock.Close()
 metrics,err:=NewMetricsClient("https://example.apps.dynatrace.com",Credentials{ClientID:"id",ClientSecret:"secret"})
 if err!=nil{t.Fatal(err)}
 metrics.OAuthURL=mock.URL
 logClient,err:=NewClient("https://example.apps.dynatrace.com",Credentials{ClientID:"id",ClientSecret:"secret"})
 if err!=nil{t.Fatal(err)}
 logClient.OAuthURL=mock.URL
 _,err=metrics.bearer(context.Background())
 if err!=nil{t.Fatal(err)}
 if scope!="storage:buckets:read storage:metrics:read"{t.Fatalf("wrong metrics scopes: %q",scope)}
 _,err=logClient.bearer(context.Background())
 if err!=nil{t.Fatal(err)}
 if scope!="storage:buckets:read storage:logs:read"{t.Fatalf("logs scope expanded: %q",scope)}
}
func TestQueryDQLRunsMetricsInsteadOfFetchLogs(t *testing.T){
 var query string
 mock:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  var payload map[string]any
  if err:=json.NewDecoder(r.Body).Decode(&payload);err!=nil{t.Error(err)}
  query,_=payload["query"].(string)
  _,_=w.Write([]byte(`{"state":"SUCCEEDED","result":{"records":[]}}`))
 }))
 defer mock.Close()
 c,err:=NewClient(mock.URL,Credentials{Token:"test"})
 if err!=nil{t.Fatal(err)}
 c.HTTP=mock.Client()
 start:=time.Now().Add(-5*time.Minute)
 _,err=c.QueryDQL(context.Background(),"timeseries cpu=sum(dt.kubernetes.container.cpu_usage)",start,time.Now(),100)
 if err!=nil{t.Fatal(err)}
 if !strings.HasPrefix(query,"timeseries cpu="){t.Fatalf("metrics query lost: %s",query)}
}
