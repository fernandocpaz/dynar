package app

// bareCommandDefaults applies the operator's preferred namespace, cluster and
// monitoring mode ONLY when dynar is launched with no arguments.
// Explicit invocations keep their existing flag semantics unchanged.
func bareCommandDefaults(args []string) []string {
 if len(args) != 0 { return args }
 return []string{"-n", "apollo", "--cluster", "tkgs-prod", "--monitor"}
}
