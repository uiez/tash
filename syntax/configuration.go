package syntax

type Configuration struct {
	// defines global environment variables.
	Envs []Env
	// defines templates(action list) can be referenced from tasks.
	// the key is template name
	Templates map[string][]Action

	// defines tasks
	// the key is task name
	Tasks map[string]Task
}

// it's the only way to pass parameters between action/template or to commands.
// if name is empty, then value or cmd will be interpreted as key=value pairs,
// otherwise value or cmd will be treated as env value
type Env struct {
	// env name
	Name string
	// env value if name is empty,
	// otherwise it should be semicolon-separated key=value or key="value" pairs
	Value string
	// execute command and capture it's output, supports unix pipe |.
	// output format could be json(map(string,string)) or key=value lines.
	Cmd string
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

	// task arguments(environment)
	Args []TaskArgument
	// a sequence of task actions.
	Actions []Action
}

type Action struct {
	contextActions
	flowActions
	fsActions
	processActions
	refActions
}
