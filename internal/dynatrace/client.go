package dynatrace

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/url"
 "strconv"
 "strings"
 "sync"
 "time"
)

const queryBase="/platform/storage/query/v1/"
const oauthURL="https://sso.dynatrace.com/sso/oauth2/token"

type Credentials struct {
 Token string
 ClientID string
 ClientSecret string
}
type Client struct {
 BaseURL string
 HTTP *http.Client
 Credentials Credentials
 OAuthURL string
 OAuthScopes string // separate metrics client never expands log-only privileges
 mu sync.Mutex
 accessToken string
 expires time.Time
}
func ValidateURL(base string)error{
 u,err:=url.Parse(base)
 if err!=nil{return err}
 if u.User!=nil || u.Hostname()=="" || u.RawQuery!="" || u.Fragment!="" || (u.Path!="" && u.Path!="/"){
  return errors.New("invalid Dynatrace base URL: use https://TENANT.apps.dynatrace.com")
 }
 if u.Scheme=="https"{return nil}
 ip:=net.ParseIP(u.Hostname())
 if u.Scheme=="http" && (strings.EqualFold(u.Hostname(),"localhost") || ip!=nil && ip.IsLoopback()){return nil}
 return errors.New("Dynatrace URL must use HTTPS")
}
func NewClient(base string,credentials Credentials)(*Client,error){
 if err:=ValidateURL(base);err!=nil{return nil,err}
 if credentials.Token=="" && (credentials.ClientID==""||credentials.ClientSecret==""){
  return nil,errors.New("no credentials: set DYNAR_TOKEN (platform/bearer token) or DYNAR_CLIENT_ID and DYNAR_CLIENT_SECRET")
 }
 u,_:=url.Parse(base);u.Path="";u.RawPath=""
 return &Client{BaseURL:strings.TrimRight(u.String(),"/"),HTTP:&http.Client{Timeout:35*time.Second},Credentials:credentials,OAuthURL:oauthURL},nil
}
// NewMetricsClient keeps the log-only OAuth credential flow unchanged.
// A separate short-lived token is requested only when the user enables M/F9.
func NewMetricsClient(base string,credentials Credentials)(*Client,error){
 client,err:=NewClient(base,credentials)
 if err!=nil{return nil,err}
 client.OAuthScopes="storage:buckets:read storage:metrics:read"
 return client,nil
}
func(c *Client)bearer(ctx context.Context)(string,error){
 if c.Credentials.Token!="" {return c.Credentials.Token,nil}
 c.mu.Lock();defer c.mu.Unlock()
 if c.accessToken!="" && time.Until(c.expires)>45*time.Second{return c.accessToken,nil}
 form:=url.Values{}
 form.Set("grant_type","client_credentials")
 form.Set("client_id",c.Credentials.ClientID)
 form.Set("client_secret",c.Credentials.ClientSecret)
 scopes:=c.OAuthScopes
 if scopes==""{scopes="storage:buckets:read storage:logs:read"}
 form.Set("scope",scopes)
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,c.OAuthURL,strings.NewReader(form.Encode()))
 if err!=nil{return "",err}
 req.Header.Set("Content-Type","application/x-www-form-urlencoded")
 resp,err:=c.HTTP.Do(req)
 if err!=nil{return "",fmt.Errorf("OAuth token request: %w",err)}
 defer resp.Body.Close()
 body,_:=io.ReadAll(io.LimitReader(resp.Body,16384))
 if resp.StatusCode<200||resp.StatusCode>=300 {return "",fmt.Errorf("OAuth token endpoint returned HTTP %d: %s",resp.StatusCode,strings.TrimSpace(string(body)))}
 var decoded struct{AccessToken string `json:"access_token"`;ExpiresIn int `json:"expires_in"`}
 if err=json.Unmarshal(body,&decoded);err!=nil{return "",err}
 if decoded.AccessToken=="" {return "",errors.New("OAuth response omitted access_token")}
 c.accessToken=decoded.AccessToken
 c.expires=time.Now().Add(time.Duration(decoded.ExpiresIn)*time.Second)
 return c.accessToken,nil
}
type Notification struct {
 Severity string `json:"severity"`
 Message string `json:"message"`
 NotificationType string `json:"notificationType"`
}
type queryResponse struct {
 State string `json:"state"`
 RequestToken string `json:"requestToken"`
 Result struct {
  Records []map[string]any `json:"records"`
  Metadata struct {
   Grail struct {
    Notifications []Notification `json:"notifications"`
    Sampled bool `json:"sampled"`
   } `json:"grail"`
  } `json:"metadata"`
 } `json:"result"`
 Error json.RawMessage `json:"error"`
}
type Result struct {
 Records []map[string]any
 Warnings []string
 AtLimit bool
}
func(c *Client)call(ctx context.Context, method,path string,payload any)(queryResponse,error){
 var body []byte
 if payload!=nil {
  var err error;body,err=json.Marshal(payload);if err!=nil{return queryResponse{},err}
 }
 var last error
 for attempt:=0;attempt<3;attempt++ {
  if attempt>0{
   timer:=time.NewTimer(time.Duration(1<<uint(attempt-1))*time.Second)
   select {case <-ctx.Done():timer.Stop();return queryResponse{},ctx.Err();case <-timer.C:}
  }
  token,err:=c.bearer(ctx)
  if err!=nil{return queryResponse{},err}
  req,err:=http.NewRequestWithContext(ctx,method,c.BaseURL+queryBase+path,bytes.NewReader(body))
  if err!=nil{return queryResponse{},err}
  req.Header.Set("Authorization","Bearer "+token)
  req.Header.Set("Accept","application/json")
  if payload!=nil{req.Header.Set("Content-Type","application/json")}
  resp,err:=c.HTTP.Do(req)
  if err!=nil{last=err;continue}
  raw,readErr:=io.ReadAll(io.LimitReader(resp.Body,32<<20))
  resp.Body.Close()
  if readErr!=nil{return queryResponse{},readErr}
  if resp.StatusCode==429||resp.StatusCode==502||resp.StatusCode==503||resp.StatusCode==504 {
   if n,err:=strconv.Atoi(resp.Header.Get("Retry-After"));err==nil&&n>0&&n<=10 {
    timer:=time.NewTimer(time.Duration(n)*time.Second)
    select {case <-ctx.Done():timer.Stop();return queryResponse{},ctx.Err();case <-timer.C:}
   }
   last=fmt.Errorf("Dynatrace HTTP %d",resp.StatusCode)
   continue
  }
  if resp.StatusCode<200||resp.StatusCode>=300 {
   msg:=strings.TrimSpace(string(raw));if len(msg)>800{msg=msg[:800]}
   return queryResponse{},fmt.Errorf("Dynatrace HTTP %d: %s",resp.StatusCode,msg)
  }
  var out queryResponse
  if err=json.Unmarshal(raw,&out);err!=nil{return queryResponse{},fmt.Errorf("decode Dynatrace response: %w",err)}
  return out,nil
 }
 return queryResponse{},fmt.Errorf("Dynatrace unavailable after retries: %w",last)
}
func DQL(namespace,cluster string,limit int)string{
 filter:="fetch logs\n| filter k8s.namespace.name == "+strconv.Quote(namespace)
 if cluster!=""{filter+="\n| filter k8s.cluster.name == "+strconv.Quote(cluster)}
 filter+="\n| fields timestamp, content, loglevel, k8s.namespace.name, k8s.workload.name, k8s.deployment.name, k8s.statefulset.name, k8s.daemonset.name, k8s.job.name, k8s.cronjob.name, k8s.pod.name, k8s.container.name, dt.process.name, trace_id, span_id, request_id, id, http.request.method, http.method, http.response.status_code, http.status_code, url.path, http.route, http.target, duration, http.server.request.duration, duration_ms, elapsed_ms\n| sort timestamp asc\n| limit "+strconv.Itoa(limit)
 return filter
}
func(c *Client)Query(ctx context.Context,namespace,cluster string,start,end time.Time,limit int)(Result,error){
 return c.QueryDQL(ctx,DQL(namespace,cluster,limit),start,end,limit)
}
// QueryDQL runs a caller-supplied DQL query with the same bounded execution
// and polling semantics as the logs collector. Never interpolate user input
// into DQL without quoting/escaping it first.
func(c *Client)QueryDQL(ctx context.Context,query string,start,end time.Time,limit int)(Result,error){
 if !end.After(start){return Result{},errors.New("invalid query time window")}
 if limit<1{return Result{},errors.New("limit must be positive")}
 payload:=map[string]any{
  "query":query,
  "defaultTimeframeStart":start.UTC().Format(time.RFC3339Nano),
  "defaultTimeframeEnd":end.UTC().Format(time.RFC3339Nano),
  "maxResultRecords":limit,
  "maxResultBytes":16<<20,
  "requestTimeoutMilliseconds":1000,
  "pollingPromiseSeconds":30,
  "includeTypes":false,
 }
 result,err:=c.call(ctx,http.MethodPost,"query:execute",payload)
 if err!=nil{return Result{},err}
 requestToken:=result.RequestToken
 deadline:=time.NewTimer(90*time.Second);defer deadline.Stop()
 for result.State=="RUNNING"||result.State=="NOT_STARTED"{
  if requestToken==""{return Result{},errors.New("Dynatrace returned running query without requestToken")}
  select {
   case <-deadline.C:return Result{},errors.New("Dynatrace query timed out after 90s")
   case <-ctx.Done():return Result{},ctx.Err()
   case <-time.After(time.Second):
  }
  values:=url.Values{}
  values.Set("request-token",requestToken)
  values.Set("request-timeout-milliseconds","5000")
  result,err=c.call(ctx,http.MethodGet,"query:poll?"+values.Encode(),nil)
  if err!=nil{return Result{},err}
 }
 if result.State!="SUCCEEDED"{return Result{},fmt.Errorf("Dynatrace query %s: %s",result.State,strings.TrimSpace(string(result.Error)))}
 out:=Result{Records:result.Result.Records,AtLimit:len(result.Result.Records)>=limit}
 if result.Result.Metadata.Grail.Sampled {out.Warnings=append(out.Warnings,"Dynatrace sampled data; results are incomplete")}
 for _,n:=range result.Result.Metadata.Grail.Notifications {
  if strings.EqualFold(n.Severity,"WARN")||strings.EqualFold(n.Severity,"ERROR"){
   msg:=n.Message;if msg==""{msg=n.NotificationType}
   out.Warnings=append(out.Warnings,msg)
   upper:=strings.ToUpper(n.NotificationType+" "+msg)
   if strings.Contains(upper,"LIMIT")||strings.Contains(upper,"TRUNCAT")||strings.Contains(upper,"RESULT")||strings.Contains(upper,"BYTE"){out.AtLimit=true}
  }
 }
 return out,nil
}
