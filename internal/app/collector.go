package app

import (
 "context"
 "fmt"
 "time"
 "github.com/fernandocpaz/dynar/internal/dynatrace"
 "github.com/fernandocpaz/dynar/internal/logs"
)

type QueryFunc func(context.Context,string,string,time.Time,time.Time,int)(dynatrace.Result,error)
type Collector struct {
 Query QueryFunc
 Limit int
 MaxQueries int
}
// Load recursively subdivides saturated intervals so the service is not restricted
// to an arbitrary number of records from the first query. Query budget exhaustion
// produces explicit warnings instead of silently returning incomplete results.
func(c Collector)Load(ctx context.Context,source,namespace,cluster string,start,end time.Time)([]logs.Event,[]string,error){
 remaining:=c.MaxQueries
 if c.Limit<1 || remaining<1 {return nil,nil,fmt.Errorf("invalid collector limits")}
 var events []logs.Event
 var warnings []string
 var queryWindow func(time.Time,time.Time)error
 queryWindow=func(from,to time.Time)error{
  if ctx.Err()!=nil{return ctx.Err()}
  if remaining==0 {
   warnings=append(warnings,fmt.Sprintf("query budget exhausted: missing records possible between %s and %s",from.Format(time.RFC3339),to.Format(time.RFC3339)))
   return nil
  }
  remaining--
  result,err:=c.Query(ctx,namespace,cluster,from,to,c.Limit)
  if err!=nil{return err}
  if result.AtLimit && to.Sub(from)>=time.Second && remaining>=2 {
   mid:=from.Add(to.Sub(from)/2)
   if err:=queryWindow(from,mid);err!=nil{return err}
   return queryWindow(mid,to)
  }
  if result.AtLimit {
   warnings=append(warnings,fmt.Sprintf("result limit reached for %s to %s; some logs may be missing (increase --limit or --max-queries)",from.Format(time.RFC3339Nano),to.Format(time.RFC3339Nano)))
  }
  warnings=append(warnings,result.Warnings...)
  for _,rec:=range result.Records {
   e,err:=logs.FromRecord(rec,source)
   if err!=nil {warnings=append(warnings,"skipped record with invalid timestamp");continue}
   if e.Timestamp.Before(from)||!e.Timestamp.Before(to){continue}
   events=append(events,e)
  }
  return nil
 }
 err:=queryWindow(start,end)
 return events,warnings,err
}
