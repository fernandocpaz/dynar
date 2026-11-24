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

const exampleTrace="0ac977cd66f5934ca76eb96044894aa7"
func TestSpansDQLExactTraceAndRejectsInjection(t *testing.T){
 q,err:=SpansDQL(exampleTrace)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(q,"fetch spans")||!strings.Contains(q,exampleTrace)||!strings.Contains(q,"span.parent_id")||strings.Contains(q,"filter k8s."){
  t.Fatalf("incorrect trace query: %s",q)
 }
 if _,err:=SpansDQL(exampleTrace+"\" | fetch logs");err==nil{t.Fatal("invalid trace accepted")}
}
func TestQuerySpansUsesActualTimingAndPermissions(t *testing.T){
 anchor:=time.Date(2026,11,24,9,11,32,0,time.UTC)
 start:=anchor.Add(-100*time.Millisecond)
 var gotQuery,gotStart,gotEnd string
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("Authorization")!="Bearer fixture"{t.Error("missing bearer")}
  var p map[string]any
  if err:=json.NewDecoder(r.Body).Decode(&p);err!=nil{t.Error(err)}
  gotQuery,_=p["query"].(string);gotStart,_=p["defaultTimeframeStart"].(string);gotEnd,_=p["defaultTimeframeEnd"].(string)
  rows:=[]map[string]any{
   {"trace.id":exampleTrace,"span.id":"1111111111111111","span.name":"POST /encounter","start_time":start.Format(time.RFC3339Nano),"end_time":anchor.Format(time.RFC3339Nano),"service.name":"encounter"},
   {"trace.id":exampleTrace,"span.id":"2222222222222222","span.parent_id":"1111111111111111","span.name":"SQL","span.status_code":"error","start_time":start.Add(10*time.Millisecond).Format(time.RFC3339Nano),"end_time":start.Add(50*time.Millisecond).Format(time.RFC3339Nano),"db.system":"mssql"},
  }
  _=json.NewEncoder(w).Encode(map[string]any{"state":"SUCCEEDED","result":map[string]any{"records":rows}})
 }))
 defer server.Close()
 c,err:=NewSpansClient(server.URL,Credentials{Token:"fixture"});if err!=nil{t.Fatal(err)}
 if c.OAuthScopes!="storage:buckets:read storage:spans:read"{t.Fatal("wrong OAuth permissions")}
 result,err:=c.QuerySpans(context.Background(),exampleTrace,anchor)
 if err!=nil{t.Fatal(err)}
 if len(result.Spans)!=2||result.Spans[1].ParentID!=result.Spans[0].ID||result.Spans[1].End.Sub(result.Spans[1].Start)!=40*time.Millisecond{t.Fatalf("span timing or hierarchy lost: %+v",result)}
 if !strings.Contains(gotQuery,exampleTrace)||gotStart!=anchor.Add(-30*time.Minute).Format(time.RFC3339Nano)||gotEnd!=anchor.Add(30*time.Minute).Format(time.RFC3339Nano){t.Fatal("query is not anchored on log time")}
}
func TestSpansRejectMalformedAndUnrelatedRows(t *testing.T){
 rows:=[]map[string]any{
  {"trace.id":exampleTrace,"span.id":"1111111111111111","start_time":"invalid"},
  {"trace.id":"ffffffffffffffffffffffffffffffff","span.id":"2222222222222222"},
 }
 result,n:=parseSpans(rows,exampleTrace)
 if len(result)!=0||n!=2{t.Fatal("malformed or unrelated span rendered")}
}
