package ui

import (
    "context"
    "strings"
    "testing"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/fernandocpaz/dynar/internal/update"
)
func TestAutoUpdateOfferDefaultsToLater(t *testing.T){
    m:=newModel(context.Background(),Config{Source:"prod",Namespace:"apollo",Updates:update.New("v0.2.0")})
    msg:=updateOfferMsg{release:update.Release{Version:"v0.3.0"},available:true}
    result,_:=m.Update(msg)
    shown:=result.(model)
    if !shown.updatePending||shown.updateFocus {t.Fatalf("unexpected update selection: %+v",shown)}
    if !strings.Contains(shown.View(),"Update & restart"){t.Fatal("missing update prompt")}
    result,_=shown.Update(tea.KeyMsg{Type:tea.KeyEsc})
    dismissed:=result.(model)
    if dismissed.updatePending{t.Fatal("Esc must dismiss update")}
}
func TestUpdateFailedRemainsInModal(t *testing.T){
    m:=newModel(context.Background(),Config{Updates:update.New("v0.2.0")})
    m.updatePending=true
    result,_:=m.Update(updateInstalledMsg{err:context.DeadlineExceeded})
    if !result.(model).updatePending||result.(model).updateBusy{t.Fatal("failed update should preserve modal, not claim success")}
    if !strings.Contains(result.(model).updateError,"deadline"){t.Fatal("expected failure shown")}
}
