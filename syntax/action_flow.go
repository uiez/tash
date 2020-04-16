package syntax

// flow control actions
type flowActions struct {
	// conditional running
	Condition ActionCondition
	// sugar for condition running
	Switch ActionSwitch
	// sugar for condition running
	If ActionIf
	// loop running, same as 'for' keyword in programming, 'while' doesn't supported yet.
	Loop ActionLoop
}

// conditional running
type ActionCondition struct {
	// a sugar syntax for a single case to reduce yaml nesting level.
	*ConditionCase `yaml:"case"`

	Cases []ConditionCase
}

// a condition case to be run
type ConditionCase struct {
	// condition checking, defined as embed to reduce yaml nesting level
	*Condition
	// a default case, if no any other cases matched, the default case will be run
	// default can be true even if condition exist.
	Default bool
	Actions []Action
}

// condition checking, support if, not, and, or
type Condition struct {
	// condition checking like 'if' keyword in programming.
	// defined as embed to reduce yaml nesting level
	*ConditionIf `yaml:"if"`

	Not *Condition
	And []Condition
	Or  []Condition
}

// if checking, if both operator and compare is empty, value will be treated as a boolean,
// if only operator is empty, it's treated as a string-equals checking
type ConditionIf struct {
	// left-hand operand
	Value string
	// operator
	Operator string
	// right-hand operand, some boolean operators doesn't needs this field
	Compare string
}

// sugar for condition checking
type ActionSwitch struct {
	Value    string
	Operator string
	Cases    []SwitchCase
}

// sugar for condition checking
type ActionIf struct {
	*Condition
	OK   []Action
	Fail []Action
}

type SwitchCase struct {
	Compare *string
	Default bool
	Actions []Action
}

// loop running
type ActionLoop struct {
	// env name to access loop variable
	Env string
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
	Actions []Action
}
