package logs

import (
    "encoding/json"
    "fmt"
    "regexp"
    "strings"
    "unicode"
)

// DynaR follows tailg's severity classification, JSON rendering, and
// condensed structured-property display without modifying stored raw logs.
var (
    severityPrefix = regexp.MustCompile("(?i)(?:^|\\[|\\s)(ERR|ERROR|FTL|FATAL|CRIT|CRITICAL|WRN|WARN|WARNING|INF|INFO|INFORMATION|DBG|DEBUG|VRB|VERBOSE|TRACE|TRC)(?:\\]|\\s|:|$)")
    kubeReplica = regexp.MustCompile("(?i)^(.+)-[a-z0-9]{8,12}-[a-z0-9]{5}$")
    kubeStateful = regexp.MustCompile("^(.+)-[0-9]+$")
    escapeSequence = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07]*(?:\x07|\x1b\\\\))")
)

func SafeText(input string) string {
    input = escapeSequence.ReplaceAllString(input, "")
    return strings.Map(func(r rune) rune {
        if r == '\n' || r == '\t' {return r}
        if r < 32 || r == 127 || unicode.Is(unicode.Cf, r) {return -1}
        return r
    }, input)
}

func WorkloadFromPod(pod string) string {
    if m := kubeReplica.FindStringSubmatch(pod); len(m)>1{return m[1]}
    if m := kubeStateful.FindStringSubmatch(pod); len(m)>1{return m[1]}
    return pod
}

func LevelCode(level string) string {
    switch strings.ToUpper(strings.TrimSpace(level)) {
    case "ERR","ERROR":return "ERR"
    case "FTL","FATAL","CRIT","CRITICAL":return "FTL"
    case "WRN","WARN","WARNING":return "WRN"
    case "INF","INFO","INFORMATION":return "INF"
    case "DBG","DEBUG":return "DBG"
    case "VRB","VERBOSE":return "VRB"
    case "TRC","TRACE":return "TRC"
    default:
        if level=="" {return "INF"}
        text:=strings.ToUpper(level)
        if len(text)>3{return text[:3]}
        return text
    }
}

func IsError(level string)bool {x:=LevelCode(level);return x=="ERR"||x=="FTL"}

func jsonField(obj map[string]any, names ...string) string {
    for _, name := range names {
        if val,ok:=obj[name];ok && val!=nil {
            switch v:=val.(type) {
            case string:if strings.TrimSpace(v)!=""{return v}
            case float64:return fmt.Sprint(v)
            }
        }
    }
    return ""
}
func structured(message string)(map[string]any,bool) {
    var obj map[string]any
    if json.Unmarshal([]byte(message),&obj)!=nil || obj==nil{return nil,false}
    return obj,true
}
func GuessLevel(message string)string {
    if obj,ok:=structured(message);ok {
        if level:=jsonField(obj,"loglevel","level","lvl","@l","severity");level!=""{return strings.ToUpper(level)}
    }
    if m:=severityPrefix.FindStringSubmatch(message);len(m)>1 {return strings.ToUpper(m[1])}
    return "INFO"
}

func stripTrailingProps(message string) string {
    // Preserve logged HTTP response bodies; only trailing JSON property maps
    // without a body/payload label are hidden, as in tailg's concise mode.
    i:=strings.LastIndex(message," {")
    if i<0{return message}
    prefix:=strings.ToLower(strings.TrimSpace(message[:i]))
    if strings.HasSuffix(prefix,"body:")||strings.HasSuffix(prefix,"payload:")||strings.HasSuffix(prefix,"response body:"){return message}
    var obj map[string]any
    if json.Unmarshal([]byte(strings.TrimSpace(message[i+1:])),&obj)==nil && obj!=nil {
        return strings.TrimRight(message[:i]," ")
    }
    return message
}

// PrettyMessage extracts the human-readable content from common Serilog /
// Microsoft JSON formats. Raw Event.Message is always preserved for search.
func PrettyMessage(e Event) []string {
    raw:=SafeText(e.Message)
    if obj,ok:=structured(raw);ok {
        rendered:=jsonField(obj,"RenderedMessage","@m","msg","message","@mt")
        if rendered==""{rendered=raw}
        logger:=jsonField(obj,"SourceContext","sourceContext","logger")
        if logger!=""{rendered="["+SafeText(logger)+"] "+rendered}
        result:=[]string{SafeText(rendered)}
        if exception:=jsonField(obj,"Exception","exception","@x");exception!=""{
            result=append(result,SafeText(exception))
        }
        return result
    }
    return []string{stripTrailingProps(raw)}
}
