package dynatrace

import (
 "context"
 "encoding/hex"
 "fmt"
 "strconv"
 "strings"
 "time"

 "github.com/fernandocpaz/tailg/pkg/logui"
)

const spanLimit=5000
func NewSpansClient(base string,credentials Credentials)(*Client,error){
 c,err:=NewClient(base,credentials)
 if err!=nil{return nil,err}
 c.OAuthScopes="storage:buckets:read storage:spans:read"
 return c,nil
}
func validHexID(s string,bytes int)bool{
 if len(s)!=bytes*2{return false}
 decoded,err:=hex.DecodeString(s)
 if err!=nil{return false}
 for _,b:=range decoded{if b!=0{return true}}
 return false
}
func SpansDQL(id string)(string,error){
 if !validHexID(id,16){return "",fmt.Errorf("invalid trace ID")}
 // UID comparison uses the native Grail UID type. No namespace/cluster
 // restriction: a distributed trace may include several environments.
 return "fetch spans\n| filter trace.id == toUid("+strconv.Quote(strings.ToLower(id))+")"+
  "\n| fields trace.id, span.id, span.parent_id, span.name, span.kind, span.status_code, span.status_message, start_time, end_time, duration, service.name, k8s.workload.name, endpoint.name, http.request.method, http.response.status_code, url.path, http.route, db.system, db.system.name, messaging.system"+
  "\n| sort start_time asc\n| limit "+strconv.Itoa(spanLimit),nil
}
func spanText(row map[string]any,keys ...string)string{
 for _,key:=range keys{
  switch v:=row[key].(type){
  case string:if v!=""{return v}
  case float64:return strconv.FormatFloat(v,'f',-1,64)
  }
 }
 return ""
}
func spanTime(v any)(time.Time,error){
 if s,ok:=v.(string);ok{
  if at,err:=time.Parse(time.RFC3339Nano,s);err==nil{return at,nil}
  if ns,err:=strconv.ParseInt(s,10,64);err==nil{return time.Unix(0,ns).UTC(),nil}
 }
 // Native API timestamps are usually RFC3339 strings. JSON numeric
 // nanoseconds may already have lost precision in float64; prefer strings.
 if n,ok:=v.(float64);ok&&n>0&&n<9.22e18{return time.Unix(0,int64(n)).UTC(),nil}
 return time.Time{},fmt.Errorf("invalid span timestamp")
}
func parseSpans(records []map[string]any,traceID string)([]logui.Span,int){
 out:=make([]logui.Span,0,len(records))
 seen:=make(map[string]bool);invalid:=0
 for _,row:=range records{
  id:=strings.ToLower(spanText(row,"span.id"))
  if !validHexID(id,8)||!strings.EqualFold(spanText(row,"trace.id"),traceID){invalid++;continue}
  if seen[id]{continue}
  start,err:=spanTime(row["start_time"]);if err!=nil{invalid++;continue}
  end,err:=spanTime(row["end_time"]);if err!=nil||end.Before(start){invalid++;continue}
  parent:=strings.ToLower(spanText(row,"span.parent_id"))
  if !validHexID(parent,8){parent=""}
  name:=spanText(row,"span.name","endpoint.name");if name==""{name="(unnamed span)"}
  out=append(out,logui.Span{
   ID:id,ParentID:parent,Name:name,Service:spanText(row,"service.name","k8s.workload.name"),
   Kind:spanText(row,"span.kind"),Status:spanText(row,"span.status_code"),
   StatusMessage:spanText(row,"span.status_message"),Start:start,End:end,
   HTTPStatus:spanText(row,"http.response.status_code"),Method:spanText(row,"http.request.method"),
   Path:spanText(row,"http.route","url.path"),DBSystem:spanText(row,"db.system.name","db.system"),
   MessagingSystem:spanText(row,"messaging.system"),
  })
  seen[id]=true
 }
 return out,invalid
}
func(c *Client)QuerySpans(ctx context.Context,id string,at time.Time)(logui.SpanResult,error){
 q,err:=SpansDQL(id);if err!=nil{return logui.SpanResult{},err}
 if at.IsZero(){at=time.Now()}
 result,err:=c.QueryDQL(ctx,q,at.Add(-30*time.Minute),at.Add(30*time.Minute),spanLimit)
 if err!=nil{return logui.SpanResult{},fmt.Errorf("span query failed (check storage:spans:read): %w",err)}
 spans,invalid:=parseSpans(result.Records,id)
 warnings:=append([]string(nil),result.Warnings...)
 if result.AtLimit{warnings=append(warnings,"span result limit reached; trace may be incomplete")}
 if invalid>0{warnings=append(warnings,fmt.Sprintf("%d invalid span rows omitted",invalid))}
 return logui.SpanResult{Spans:spans,Warning:strings.Join(warnings,"; ")},nil
}
