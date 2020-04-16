package syntax

// process actions
type processActions struct {
	// kill/signal process
	Pkill ActionPkill
	// sleep ms
	Sleep ActionSleep
	// execute command
	Cmd ActionCmd
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
}

// pkill process
type ActionPkill struct {
	Process string
	Pid     int
	Signal  string
}

// sleep ms
type ActionSleep uint
