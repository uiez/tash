package syntax

// process actions
type processActions struct {
	// kill/signal process
	Pkill ActionPkill
	// sleep ms
	Sleep ActionSleep
	// execute command
	Cmd ActionCmd
	// wait process exit
	Wait ActionWait
}

// command execution
type ActionCmd struct {
	// command line string, supports unix pipe
	Exec string

	// io redirection from/to file

	// os.Stdin if empty
	Stdin string

	// os.Stdout if empty
	Stdout string
	// append to or truncate file
	StdoutAppend bool

	// os.Stderr if empty
	Stderr       string
	StderrAppend bool

	// run in background
	Background bool
}

// pkill process
type ActionPkill struct {
	Process string
	Pid     string
	Signal  string
}

// sleep ms
type ActionSleep uint

// wait process execution finish
type ActionWait struct {
	Process string
	Pid     string
}
