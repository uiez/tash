package syntax

// reference actions
type refActions struct {
	// execute actions defined in template
	Template ActionTemplate
	// run task
	Task ActionTask
}

// run another task
type ActionTask struct {
	Name       string
	PassEnvs   []string
	ReturnEnvs []string
}

// run actions defined in template
type ActionTemplate = string
