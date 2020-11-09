package syntax

// flow control actions
type flowActions struct {
	// sugar for condition running
	Switch ActionSwitch
	// sugar for condition running
	If ActionIf
	// loop running, same as 'for' keyword in programming, 'while' doesn't supported yet.
	Loop ActionLoop
}

// sugar for condition checking
type ActionSwitch struct {
	Value    string
	Operator string
	Default  string
	Cases    map[string]ActionList
}

// sugar for condition checking
type ActionIf struct {
	Check string

	Actions ActionList
	Else    ActionList
}

// loop running
type ActionLoop struct {
	// env name to access loop variable
	Var string
	// loop by times
	Times int
	// loop in range, from,to,step could be both negative
	Seq struct {
		From, To, Step int
	}
	// loop over string array
	Array []string
	// loop over string array split from given value and separator
	Split struct {
		Value     string
		Separator string
	}

	// actions to be run
	Actions ActionList
}
