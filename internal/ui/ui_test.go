package ui

import (
    "context"
    "strings"
    "testing"
    "time"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/fernandocpaz/dynar/internal/logs"
)
func makeTestModel() model{
    m:=newModel(context.Background(),Config{Source:"prod",Namespace:"apollo",Refresh:time.Second,Since:time.Hour})
    m.width=140;m.height=38
    m.store.Add([]logs.Event{
        {Identity:"1",Timestamp:time.Now(),Service:"encounter",Level:"ERROR",Message:"SQL timeout",Pod:"encounter-123"},
        {Identity:"2",Timestamp:time.Now(),Service:"patient",Level:"INFO",Message:"Lookup completed",Pod:"patient-123"},
        {Identity:"3",Timestamp:time.Now(),Service:"encounter",Level:"INFO",Message:"Saved"},
    })
    m.rebuild("")
    m.loading=false
    return m
}
func TestGridGroupsServices(t *testing.T){
    m:=makeTestModel()
    if len(m.services)!=2 {t.Fatalf("want two service panels, got %v",m.services)}
    v:=m.View()
    if !strings.Contains(v,"encounter")||!strings.Contains(v,"patient"){t.Fatalf("missing service groups: %s",v)}
    if !strings.Contains(v,"SQL timeout")||!strings.Contains(v,"Lookup completed"){t.Fatalf("missing service records: %s",v)}
}
func TestSearchAcrossAllPanels(t *testing.T){
    m:=makeTestModel()
    m.search="timeout";m.onlyMatches=true
    if !m.visible(m.groups["encounter"][0]){t.Fatal("expected matching event")}
    if m.visible(m.groups["patient"][0]){t.Fatal("unrelated event should be hidden")}
    m.errorsOnly=true
    if m.visible(m.groups["encounter"][1]){t.Fatal("INFO should be hidden")}
    m.combined=true;m.focus=true
    if !strings.Contains(m.View(),"ALL SERVICES"){t.Fatal("missing combined timeline")}
}
func TestNavigation(t *testing.T){
    m:=makeTestModel()
    next,_:=m.Update(tea.KeyMsg{Type:tea.KeyTab})
    if next.(model).selected!=1{t.Fatal("Tab should select next service")}
    focused,_:=next.(model).Update(tea.KeyMsg{Type:tea.KeyEnter})
    if !focused.(model).focus{t.Fatal("Enter should maximize panel")}
    back,_:=focused.(model).Update(tea.KeyMsg{Type:tea.KeyEsc})
    if back.(model).focus{t.Fatal("Esc should restore grid")}
}
func TestFormatting(t *testing.T){
    e:=logs.Event{Timestamp:time.Now(),Service:"patient",Level:"Error",Message:"{\"@m\":\"Exception while saving\",\"@x\":\"at Save()\",\"@l\":\"Error\"}"}
    lines:=formatEvent(e,40,false,false)
    combined:=strings.Join(lines,"\n")
    if !strings.Contains(combined,"[ERR]")||!strings.Contains(combined,"Exception while")||!strings.Contains(combined,"at Save()"){t.Fatalf("bad format %q",combined)}
    if len(wrap(strings.Repeat("a",100),20))!=5{t.Fatal("must wrap full lines")}
}
