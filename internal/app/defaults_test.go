package app

import (
 "reflect"
 "testing"
)

func TestBareDynarRunsApolloProdWithMonitor(t *testing.T) {
 want := []string{"-n", "apollo", "--cluster", "tkgs-prod", "--monitor"}
 if got := bareCommandDefaults(nil); !reflect.DeepEqual(got, want) {
  t.Fatalf("bare dynar args = %q, want %q", got, want)
 }
 if got := bareCommandDefaults([]string{}); !reflect.DeepEqual(got, want) {
  t.Fatalf("empty dynar args = %q, want %q", got, want)
 }
}

func TestExplicitOptionsAreNeverOverridden(t *testing.T) {
 for _, args := range [][]string{
  {"-n", "test"},
  {"-n", "apollo", "--cluster", "tkgs-qa"},
  {"--namespace", "emrtrain", "--monitor=false"},
  {"--once"},
  {"--help"},
  {"source", "list"},
  {"version"},
 } {
  got := bareCommandDefaults(args)
  if !reflect.DeepEqual(got, args) {
   t.Fatalf("explicit arguments %q changed to %q", args, got)
  }
 }
}
