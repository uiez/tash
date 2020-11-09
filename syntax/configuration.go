package syntax

import (
	"encoding/json"
)

// Env:
//   could be text block(lines of semicolon separated key-value pair: key=value or key="value")
type EnvList struct {
	envs []string
}

func (e *EnvList) UnmarshalJSON(bytes []byte) error {
	var arrayTester []json.RawMessage
	if json.Unmarshal(bytes, &arrayTester) == nil {
		var envs []string
		err := json.Unmarshal(bytes, &envs)
		if err != nil {
			return err
		}
		e.envs = envs
	} else {
		var env string
		err := json.Unmarshal(bytes, &env)
		if err != nil {
			return err
		}
		e.envs = []string{env}
	}
	return nil
}
func (e *EnvList) Length() int {
	return len(e.envs)
}
func (e *EnvList) Envs() []string {
	return e.envs
}
func (e *EnvList) Append(es *EnvList) {
	e.envs = append(e.envs, es.envs...)
}
func (e *EnvList) AppendItem(s string) {
	e.envs = append(e.envs, s)
}

type ActionList struct {
	actions []Action
}

func (a *ActionList) UnmarshalJSON(bytes []byte) error {
	var arrayTester []json.RawMessage
	if json.Unmarshal(bytes, &arrayTester) == nil {
		var actions []Action
		err := json.Unmarshal(bytes, &actions)
		if err != nil {
			return err
		}
		a.actions = actions
	} else {
		var action Action
		err := json.Unmarshal(bytes, &action)
		if err != nil {
			return err
		}
		a.actions = []Action{action}
	}
	return nil
}
func (a *ActionList) Length() int {
	return len(a.actions)
}
func (a *ActionList) Actions() []Action {
	return a.actions
}

type Configuration struct {
	// import other config files, supports path globbing, can be both absolute or relative path.
	// relative path is based on current file directory.
	// supports import tash config file(.yaml,.yml) and environment config file(.env)
	//
	// directories will be ignored
	Imports string

	// defines global environment variables.
	Env EnvList
	// defines templates(action list) can be referenced from tasks.
	// the key is template name
	Templates map[string]ActionList

	// defines tasks
	// the key is task name
	Tasks map[string]Task
}

// defines task arguments
type TaskArgument struct {
	// task argument name as environment variable
	Env         string
	Description string
	// argument default value
	Default string
}

type Task struct {
	Description string
	// current directory if empty
	WorkDir string

	// task arguments(can be passed as environment or command line options)
	Args []TaskArgument

	// a sequence of task actions.
	Actions ActionList
}

type Action struct {
	contextActions
	flowActions
	fsActions
	processActions
	refActions
}

const DefaultArraySeparator = " "
