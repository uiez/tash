package main

// Expanding:
// almost all strings will be expanded first with environment,
// except:
//	* env name
//	* task work dir
//  * loop seq/times
//  etc..
// the expandable format are:
//	* $ENV_NAME_ALPHA_NUM
//	* ${ENV_NAME_NO_LIMIT [| filter[ arg]...]...}
//	* ${"string literal" [| filter[ arg]...]...}
// uses '\' to avoid escaping, such as '\$', '\$', '\\'
//
// predefined task-specific env:
//    WORKDIR: task initial working directory
//    TASK_NAME: task name

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

type Task struct {
	Description string
	// tasks depends on, couldn't be cycle-referenced.
	Depends []string
	// current directory if empty
	WorkDir string

	// a sequence of task actions.
	Actions []Action
}

type Action struct {
	// define environments
	Env Env
	// execute command
	Cmd ActionCmd
	// copy resources
	Copy ActionCopy
	// delete file/directory
	Del string
	// replace file content
	Replace ActionReplace
	// change file/directory mode
	Chmod ActionChmod
	// change current working directory
	Chdir ActionChdir
	// create directory and it's parents, ignore if already existed
	Mkdir string
	// execute actions defined in template
	Template string
	// conditional running
	Condition ActionCondition
	// sugar for condition running
	Switch ActionSwitch
	// loop running, same as 'for' keyword in programming, 'while' doesn't supported yet.
	Loop ActionLoop
	// silent logs or errors, same as '-' and '@' in makefile.
	Silent ActionSilent
}

const (
	ResourceHashAlgSha1   = "SHA1"
	ResourceHashAlgMD5    = "MD5"
	ResourceHashAlgSha256 = "SHA256"
)

// resource copy
type ActionCopy struct {
	// source url could be file or http/https if contains schema, otherwise it will be treated as file
	// both source and dest could be directory in file mode.
	SourceUrl string
	// if source is directory, destPath will be removed first, than copy again
	DestPath string
	// hash checking for file
	Hash struct {
		// hash algorithm, support SHA1, MD5 and SHA256, sha1 by default.
		Alg string
		// hexadecimal string, case insensitive
		Sig string
	}
}

// replace file content
type ActionReplace struct {
	// file path, not directory
	File string
	// replaces, key is old string, value is new string
	Replaces map[string]string
	// do regexp replacing
	Regexp bool
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

// change path mode such as 0644 for file, 0755 for directory and executable.
type ActionChmod struct {
	Path string
	Mode uint
}

// change current working directory
type ActionChdir struct {
	Dir string
	// actions run in new working directory
	Actions []Action
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

const (
	SilentFlagAllowError = "allowError"
	SilentFlagShowLog    = "showLog"
)

// silent execution, default hide log, but still fatal on errors
// uses flags to changes the default behavior
type ActionSilent struct {
	Flags   []string
	Actions []Action
}

// operators in condition and switch
// there is a sugar that put a op_bool_not before actual operator to do not checking.
const (
	op_bool_not                  = "bool.not"
	op_bool_true                 = "bool.true"
	op_string_greaterThan        = "string.greaterThan"
	op_string_greaterThanOrEqual = "string.greaterThanOrEqual"
	op_string_equal              = "string.equal"
	op_string_notEqual           = "string.notEqual"
	op_string_lessThanOrEqual    = "string.lessThanOrEqual"
	op_string_lessThan           = "string.lessThan"
	op_string_notEmpty           = "string.notEmpty"
	op_string_empty              = "string.empty"
	op_string_regexp             = "string.regexp"
	op_number_greaterThan        = "number.greaterThan"
	op_number_greaterThanOrEqual = "number.greaterThanOrEqual"
	op_number_equal              = "number.equal"
	op_number_notEqual           = "number.notEqual"
	op_number_lessThanOrEqual    = "number.lessThanOrEqual"
	op_number_lessThan           = "number.lessThan"
	op_env_defined               = "env.defined"
	op_file_newerThan            = "file.newerThan"
	op_file_olderThan            = "file.olderThan"
	op_file_exist                = "file.exist"
	op_file_blockDevice          = "file.blockDevice"
	op_file_charDevice           = "file.charDevice"
	op_file_dir                  = "file.dir"
	op_file_regular              = "file.regular"
	op_file_setgid               = "file.setgid"
	op_file_symlink              = "file.symlink"
	op_file_sticky               = "file.sticky"
	op_file_namedPipe            = "file.namedPipe"
	op_file_notEmpty             = "file.notEmpty"
	op_file_socket               = "file.socket"
	op_file_setuid               = "file.setuid"
)

var operatorAlias = map[string]string{
	"?":    op_bool_true,
	"!":    op_bool_not,
	"not":  op_bool_not,
	">":    op_string_greaterThan,
	">=":   op_string_greaterThanOrEqual,
	"==":   op_string_equal,
	"!=":   op_string_notEqual,
	"<=":   op_string_lessThanOrEqual,
	"<":    op_string_lessThan,
	"-n":   op_string_notEmpty,
	"-z":   op_string_empty,
	"=~":   op_string_regexp,
	"-gt":  op_number_greaterThan,
	"-ge":  op_number_greaterThanOrEqual,
	"-eq":  op_number_equal,
	"-ne":  op_number_notEqual,
	"-le":  op_number_lessThanOrEqual,
	"-lt":  op_number_lessThan,
	"-env": op_env_defined,
	"-nt":  op_file_newerThan,
	"-ot":  op_file_olderThan,
	"-a":   op_file_exist,
	"-e":   op_file_exist,
	"-b":   op_file_blockDevice,
	"-c":   op_file_charDevice,
	"-d":   op_file_dir,
	"-f":   op_file_regular,
	"-g":   op_file_setgid,
	"-h":   op_file_symlink,
	"-L":   op_file_symlink,
	"-k":   op_file_sticky,
	"-p":   op_file_namedPipe,
	"-s":   op_file_notEmpty,
	"-S":   op_file_socket,
	"-u":   op_file_setuid,
}

// expand filters
const (
	// args: defaultValue
	ef_stringDefault = "string.default"
	// nargs: 0: whole string, 1: lower characters after [index], 2:index count, lower [count] characters after [index]
	// index could be negative to iterate from last, begin at -1
	ef_stringLower = "string.lower"
	// args is same as stringLower, but transform to upper case.
	ef_stringUpper = "string.upper"
	// args is same as stringLower, but returns string inside the range
	ef_stringSlice = "string.slice"
	// args: [old new]..., do literal replacing
	ef_stringReplace = "string.replace"
	// args: [old new]..., do regexp replacing
	ef_stringRegexpReplace = "string.regexpReplace"
	// return files match given pattern, args: no args
	ef_fileGlob = "file.glob"
	// args: no args
	ef_fileAbspath = "file.abspath"
	// args: no args
	ef_fileDirname = "file.dirname"
	// args: no args
	ef_fileBasename = "file.basename"
)
