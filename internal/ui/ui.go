// Package ui provides a full-screen, per-workload Dynatrace log viewer.
package ui

import (
    "context"
    "fmt"
    "io"
    "regexp"
    "sort"
    "strings"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    "github.com/fernandocpaz/dynar/internal/logs"
)

// LoadFunc is the namespace-wide collection function. Filtering by service is
// deliberately done locally so one Dynatrace query feeds all panels.
type LoadFunc func(context.Context,time.Time,time.Time)([]logs.Event,[]string,error)

type Config struct {
    Source string
    Namespace string
    Cluster string
    Since time.Duration
    Refresh time.Duration
    BufferLines int
    InitialSearch string
    Regex bool
    Level string
    Service string
    Load LoadFunc
}

type loadMsg struct {
    events []logs.Event
    warnings []string
    err error
    from time.Time
    to time.Time
}
type refreshMsg struct{}
type model struct {
    ctx context.Context
    config Config
    store *logs.Store
    groups map[string][]logs.Event
    services []string
    selected int
    offsets map[string]int
    width, height int
    focus, combined bool
    search, draft string
    searching bool
    onlyMatches, errorsOnly, help bool
    pattern *regexp.Regexp
    searchError string
    loading, following bool
    lastError string
    warnings []string
    lastLoaded time.Time
    nextFrom, nextTo time.Time
}
var (
    headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light:"#176B3A",Dark:"#7EE787"})
    errorStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light:"#B42318",Dark:"#FF7777"})
    warnStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light:"#966300",Dark:"#FFD166"})
    mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light:"#656970",Dark:"#8B949E"})
    highlightStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light:"#915A00",Dark:"#FFF09B"})
)
func newModel(ctx context.Context,c Config) model {
    if c.BufferLines<=0 {c.BufferLines=20000}
    if c.Refresh<=0 {c.Refresh=10*time.Second}
    if c.Since<=0 {c.Since=30*time.Minute}
    now:=time.Now().UTC()
    m:=model{
        ctx:ctx,config:c,store:logs.NewStore(c.BufferLines),
        groups:map[string][]logs.Event{},offsets:map[string]int{},
        width:120,height:35,following:true,loading:true,
        search:c.InitialSearch, onlyMatches:c.InitialSearch!="", nextFrom:now.Add(-c.Since),nextTo:now,
    }
    m.setPattern(c.InitialSearch)
    return m
}
func Run(ctx context.Context,c Config,out io.Writer)error {
    _,err:=tea.NewProgram(newModel(ctx,c),tea.WithAltScreen(),tea.WithContext(ctx),tea.WithOutput(out)).Run()
    return err
}
func(m model) Init()tea.Cmd{return m.loadCmd()}
func(m model) loadCmd()tea.Cmd{
    from,to:=m.nextFrom,m.nextTo
    fn:=m.config.Load
    ctx:=m.ctx
    return func()tea.Msg{
        if fn==nil{return loadMsg{err:fmt.Errorf("no Dynatrace loader configured"),from:from,to:to}}
        events,warnings,err:=fn(ctx,from,to)
        return loadMsg{events:events,warnings:warnings,err:err,from:from,to:to}
    }
}
func(m model) refreshCmd()tea.Cmd{
    return tea.Tick(m.config.Refresh,func(time.Time)tea.Msg{return refreshMsg{}})
}
func(m model) Update(msg tea.Msg)(tea.Model,tea.Cmd){
    switch v:=msg.(type){
    case tea.WindowSizeMsg:
        m.width,m.height=v.Width,v.Height
        return m,nil
    case loadMsg:
        m.loading=false
        if v.err!=nil{
            m.lastError=v.err.Error()
        }else{
            m.lastError=""
            m.warnings=v.warnings
            previous:=m.selectedName()
            m.store.Add(v.events)
            m.rebuild(previous)
            m.lastLoaded=time.Now()
            m.nextFrom=v.to.Add(-5*time.Minute)
            m.nextTo=time.Now().UTC()
        }
        if m.following{return m,m.refreshCmd()}
        return m,nil
    case refreshMsg:
        if m.following && !m.loading{
            m.loading=true
            m.nextTo=time.Now().UTC()
            return m,m.loadCmd()
        }
        return m,nil
    case tea.KeyMsg:
        key:=v.String()
        if key=="ctrl+c"{return m,tea.Quit}
        if m.searching{
            switch key{
            case "esc":m.searching=false;m.draft=m.search;m.searchError=""
            case "enter":
                if m.setPattern(m.draft){
                    m.search=m.draft;m.onlyMatches=m.search!="";m.searching=false;m.searchError=""
                    m.resetOffsets()
                }else{m.searchError="Invalid regular expression"}
            case "backspace", "ctrl+h":
                if len(m.draft)>0 {r:=[]rune(m.draft);m.draft=string(r[:len(r)-1])}
            case "ctrl+u":m.draft=""
            default:
                if v.Type==tea.KeyRunes{m.draft+=string(v.Runes)}
            }
            return m,nil
        }
        if m.help{
            if key=="esc"||key=="?"||key=="q"{m.help=false}
            return m,nil
        }
        switch key{
        case "q":return m,tea.Quit
        case "?":m.help=true
        case "/":m.draft=m.search;m.searching=true;m.searchError=""
        case "c":m.search="";m.draft="";m.pattern=nil;m.onlyMatches=false;m.resetOffsets()
        case "f1":m.onlyMatches=!m.onlyMatches;m.resetOffsets()
        case "f3":m.errorsOnly=!m.errorsOnly;m.resetOffsets()
        case "a":
            m.combined=!m.combined
            m.focus=m.combined
        case "enter":
            if m.combined {m.combined=false;m.focus=false
            } else if len(m.services)>0 {m.focus=!m.focus}
        case "esc":
            m.focus=false;m.combined=false
        case "tab","right":
            if len(m.services)>0 && !m.combined{m.selected=(m.selected+1)%len(m.services)}
        case "shift+tab","left":
            if len(m.services)>0 && !m.combined{m.selected=(m.selected+len(m.services)-1)%len(m.services)}
        case "up","k":
            if m.focus||m.combined{m.scroll(1)} else if len(m.services)>0{m.selected=(m.selected+len(m.services)-1)%len(m.services)}
        case "down","j":
            if m.focus||m.combined{m.scroll(-1)} else if len(m.services)>0{m.selected=(m.selected+1)%len(m.services)}
        case "pgup":
            if m.focus||m.combined{m.scroll(max(1,m.height-8))} else if len(m.services)>0{m.selected=max(0,m.selected-m.pageSize())}
        case "pgdown":
            if m.focus||m.combined{m.scroll(-max(1,m.height-8))} else if len(m.services)>0{m.selected=min(len(m.services)-1,m.selected+m.pageSize())}
        case "home":
            if m.focus||m.combined{m.offsets[m.activeName()]=m.totalLines(m.activeEvents())} else{m.selected=0}
        case "end":
            if m.focus||m.combined{m.offsets[m.activeName()]=0} else if len(m.services)>0{m.selected=len(m.services)-1}
        case " ":
            m.following=!m.following
            if m.following && !m.loading {
                m.loading=true
                return m,m.loadCmd()
            }
        case "r":
            if !m.loading {
                m.loading=true
                m.nextTo=time.Now().UTC()
                return m,m.loadCmd()
            }
        }
    }
    return m,nil
}
func(m *model) setPattern(value string)bool{
    if value=="" {m.pattern=nil;return true}
    if !m.config.Regex {m.pattern=nil;return true}
    compiled,err:=regexp.Compile("(?i)"+value)
    if err!=nil{return false}
    m.pattern=compiled
    return true
}
func(m model) selectedName()string {
    if m.selected<0 || m.selected>=len(m.services){return ""}
    return m.services[m.selected]
}
func(m model) activeName()string{
    if m.combined{return "ALL"}
    return m.selectedName()
}
func(m model) activeEvents()[]logs.Event{
    if m.combined{return m.store.Items}
    return m.groups[m.selectedName()]
}
func(m *model) resetOffsets(){m.offsets=map[string]int{}}
func(m *model) scroll(change int){
    key:=m.activeName()
    value:=m.offsets[key]+change
    if value<0{value=0}
    if value>len(m.activeEvents()){value=len(m.activeEvents())}
    m.offsets[key]=value
}
func(m *model) rebuild(previous string){
    m.groups=map[string][]logs.Event{}
    for _,e:=range m.store.Items{
        if m.config.Service!=""&&!strings.Contains(strings.ToLower(e.Service),strings.ToLower(m.config.Service)){continue}
        m.groups[e.Service]=append(m.groups[e.Service],e)
    }
    m.services=m.services[:0]
    for name:=range m.groups{m.services=append(m.services,name)}
    sort.Strings(m.services)
    m.selected=0
    for i,name:=range m.services{
        if name==previous {m.selected=i;break}
    }
}
func(m model) matches(e logs.Event) bool {
    if m.search=="" {return false}
    value:=strings.Join([]string{e.Message,e.Service,e.Pod,e.Container,e.Level,e.TraceID,e.RequestID,e.Source,e.Namespace}," ")
    if m.pattern!=nil{return m.pattern.MatchString(value)}
    return strings.Contains(strings.ToLower(value),strings.ToLower(m.search))
}
func(m model) visible(e logs.Event)bool{
    if m.config.Level!="" && logs.LevelCode(e.Level)!=logs.LevelCode(m.config.Level){return false}
    if m.errorsOnly && !logs.IsError(e.Level){return false}
    if m.onlyMatches && m.search!="" && !m.matches(e){return false}
    return true
}
func(m model) pageSize()int{
    _,_,_,count:=m.gridDimensions()
    return count
}
func(m model) gridDimensions()(columns,rows,panelHeight,perPage int){
    columns=1
    if m.width>=105 {columns=2}
    if m.width>=165 {columns=3}
    available:=max(7,m.height-5)
    rows=max(1,available/14)
    if rows>3 {rows=3}
    panelHeight=max(7,available/rows)
    perPage=columns*rows
    return
}
func(m model) View()string{
    if m.width<34 || m.height<12{return "DynaR: enlarge terminal (minimum 34x12). Press q to quit."}
    activity:="LIVE"
    if !m.following{activity="PAUSED"}
    if m.loading{activity="QUERYING"}
    loaded:="not yet"
    if !m.lastLoaded.IsZero(){loaded=m.lastLoaded.Local().Format("15:04:05")}
    title:=fmt.Sprintf("DynaR   %s / %s   %d services  %d records   %s   updated %s",
        m.config.Source,m.config.Namespace,len(m.services),len(m.store.Items),activity,loaded)
    if m.config.Cluster!=""{title+="   cluster "+m.config.Cluster}
    title=shorten(logs.SafeText(title),m.width)
    head:=headerStyle.Render(title)
    notice:=""
    if m.lastError!=""{notice=errorStyle.Render(shorten("QUERY ERROR: "+m.lastError,m.width))}
    if notice=="" && len(m.warnings)>0{notice=warnStyle.Render(shorten("INCOMPLETE DATA: "+strings.Join(m.warnings,"; "),m.width))}
    if notice==""{notice=mutedStyle.Render("Tab select  Enter maximize  / search  F1 matches  F3 errors  A combined  Space pause  R refresh  ? help")}
    var body string
    if m.help{body=m.helpView()
    }else if m.combined{
        body=m.panel("ALL",m.store.Items,m.width,max(7,m.height-5),true)
    }else if m.focus && len(m.services)>0{
        name:=m.selectedName()
        body=m.panel(name,m.groups[name],m.width,max(7,m.height-5),true)
    }else{body=m.gridView()}
    foot:=""
    if m.searching{
        foot="SEARCH / "+m.draft+"_   (Enter apply, Esc cancel)"
        if m.config.Regex{foot+=" [REGEX]"}
        if m.searchError!=""{foot+="   "+m.searchError}
    }else{
        state:="All logs"
        if m.onlyMatches && m.search!=""{state="Matching only"}
        if m.search!=""{state+="  / "+m.search}
        if m.errorsOnly{state+="  [ERRORS ONLY]"}
        if m.focus||m.combined{state+="   ↑↓ scroll  PgUp/PgDn  Esc grid"} else {state+="   ←→ switch services"}
        foot=state
    }
    foot=shorten(logs.SafeText(foot),m.width)
    return head+"\n"+notice+"\n"+body+"\n"+mutedStyle.Render(foot)
}
func(m model) helpView()string{
    help:=[]string{
        "DynaR — Keyboard shortcuts",
        "",
        "Tab / Shift+Tab     Select previous/next service",
        "Left / Right        Select service in grid",
        "Enter               Maximize selected service; Enter again restores grid",
        "A                   Combined chronological timeline across all services",
        "/                   Search content, service, pod, trace and request ID",
        "F1                  Toggle matching records only (global across services)",
        "F3                  Toggle errors only (all panels)",
        "C                   Clear search",
        "Up/Down             Scroll messages in maximized/combined view",
        "PgUp / PgDn         Scroll view or change grid page",
        "Home / End          Oldest/newest in focus mode",
        "Space               Pause/resume Dynatrace polling",
        "R                   Refresh now",
        "? / Esc             Close help",
        "Q                   Quit",
        "",
        "All services continue to be collected by one namespace-wide query.",
        "Non-interactive / redirected output remains available with --no-ui.",
    }
    lines:=make([]string,0,m.height)
    for _,line:=range help{lines=append(lines,shorten(line,max(12,m.width-4)))}
    for len(lines)<max(7,m.height-5){lines=append(lines,"")}
    return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0,1).Width(m.width-2).Height(max(7,m.height-5)-2).Render(strings.Join(lines,"\n"))
}
func(m model) gridView()string{
    if len(m.services)==0{
        status:="Waiting for accessible namespace logs..."
        if !m.loading && m.lastError==""{status="No logs returned for the selected namespace/time range"}
        return m.panel(status,nil,m.width,max(7,m.height-5),false)
    }
    cols,rows,height,perPage:=m.gridDimensions()
    first:=(m.selected/perPage)*perPage
    last:=min(len(m.services),first+perPage)
    widths:=make([]int,cols)
    for i:=range widths {widths[i]=m.width/cols}
    widths[cols-1]+=m.width%cols
    var rendered []string
    for r:=0;r<rows;r++{
        var panels []string
        for col:=0;col<cols;col++{
            i:=first+r*cols+col
            if i<last {
                name:=m.services[i]
                panels=append(panels,m.panel(name,m.groups[name],widths[col],height,i==m.selected))
            }else{
                panels=append(panels,strings.Repeat(" ",widths[col]))
            }
        }
        rendered=append(rendered,lipgloss.JoinHorizontal(lipgloss.Top,panels...))
    }
    return strings.Join(rendered,"\n")
}
func(m model) totalLines(events []logs.Event)int{
    width:=max(12,m.width-4)
    sum:=0
    for _,e:=range events{
        if m.visible(e){sum+=len(formatEvent(e,width,m.combined,false))}
    }
    return sum
}
func(m model) panel(name string,events []logs.Event,width,height int,selected bool)string{
    inner:=max(12,width-4)
    bodyHeight:=max(3,height-3)
    visibleCount:=0
    for _,e:=range events{if m.visible(e){visibleCount++}}
    label:=name
    if selected{label="▶ "+label}
    if name=="ALL"{label="ALL SERVICES"}
    title:=fmt.Sprintf("%s  [%d/%d]",label,visibleCount,len(events))
    title=shorten(logs.SafeText(title),inner)
    if selected {title=highlightStyle.Bold(true).Render(title)} else {title=headerStyle.Render(title)}
    offset:=m.offsets[name]
    reverse:=make([]string,0,bodyHeight)
    for i:=len(events)-1;i>=0 && len(reverse)<bodyHeight;i--{
        e:=events[i]
        if !m.visible(e){continue}
        lines:=formatEvent(e,inner,name=="ALL",m.search!=""&&m.matches(e))
        for j:=len(lines)-1;j>=0 && len(reverse)<bodyHeight;j--{
            if offset>0{offset--;continue}
            reverse=append(reverse,lines[j])
        }
    }
    display:=make([]string,0,bodyHeight)
    for i:=len(reverse)-1;i>=0;i--{display=append(display,reverse[i])}
    for len(display)<bodyHeight{display=append(display,"")}
    content:=title+"\n"+strings.Join(display,"\n")
    border:=lipgloss.AdaptiveColor{Light:"#737C83",Dark:"#4B5563"}
    if selected{border=lipgloss.AdaptiveColor{Light:"#A66E00",Dark:"#E4B75B"}}
    return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
        Width(width-2).Height(height-2).Render(content)
}
func formatEvent(e logs.Event,width int,combined,matched bool)[]string{
    prefix:=e.Timestamp.Local().Format("15:04:05.000")+" ["+logs.LevelCode(e.Level)+"] "
    if combined{prefix+="["+logs.SafeText(e.Service)+"] "}
    message:=logs.PrettyMessage(e)
    var result []string
    for index,part:=range message{
        parts:=strings.Split(part,"\n")
        for sub,line:=range parts{
            lead:=""
            if index==0 && sub==0 {lead=prefix} else {lead="    "}
            for _,segment:=range wrap(lead+strings.ReplaceAll(line,"\t","    "),width){
                colored:=colorize(segment,e.Level,matched)
                result=append(result,colored)
            }
        }
    }
    return result
}
func colorize(line,level string,matched bool)string{
    if matched {return highlightStyle.Render(line)}
    switch logs.LevelCode(level){
    case "ERR","FTL":return errorStyle.Render(line)
    case "WRN":return warnStyle.Render(line)
    case "INF":return mutedStyle.Render(line)
    case "DBG","TRC","VRB":return mutedStyle.Faint(true).Render(line)
    default:return line
    }
}
// wrap keeps complete messages accessible via vertical scrolling rather than
// chopping at the terminal edge. It operates before ANSI coloring.
func wrap(line string,width int)[]string{
    if width<5{width=5}
    chars:=[]rune(line)
    if len(chars)==0{return []string{""}}
    var out []string
    for len(chars)>width{
        cut:=width
        // Prefer word breaks when available in the last third of the line.
        for i:=width-1;i>=width*2/3;i--{
            if chars[i]==' ' {cut=i+1;break}
        }
        out=append(out,string(chars[:cut]))
        chars=chars[cut:]
    }
    out=append(out,string(chars))
    return out
}
func shorten(s string,width int)string{
    if width<1{return ""}
    chars:=[]rune(s)
    if len(chars)<=width{return s}
    if width==1{return "…"}
    return string(chars[:width-1])+"…"
}
