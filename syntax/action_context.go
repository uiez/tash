package syntax

// context actions
type contextActions struct {
	// define environments
	Env ActionEnv
	// change current working directory
	Chdir ActionChdir
	// silent logs or errors, same as '-' and '@' in makefile.
	Silent ActionSilent
}

// environment definition
type ActionEnv = Env

const (
	SilentFlagAllowError = "allowError"
	SilentFlagShowLog    = "showLog"
)

// change current working directory
type ActionChdir struct {
	Dir string
	// actions run in new working directory
	Actions []Action
}

// silent execution, default hide log, but still fatal on errors
// uses flags to changes the default behavior
type ActionSilent struct {
	Flags   []string
	Actions []Action
}
