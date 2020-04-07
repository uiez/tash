package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	ResourceHashSha1   = "SHA1"
	ResourceHashMD5    = "MD5"
	ResourceHashSha256 = "SHA256"
)

type Env struct {
	Name  string
	Value string
	Cmd   string
}

type ActionCopy struct {
	SourceUrl string
	DestPath  string
	Hash      struct {
		Alg string
		Sig string
	}
}

type ActionReplace struct {
	File     string
	Replaces map[string]string
	Regexp   bool
}

type ConditionIf struct {
	Value    string
	Operator string
	Compare  string
}

type Condition struct {
	*ConditionIf `yaml:"if"`

	Not *Condition
	And []Condition
	Or  []Condition
}

type ConditionCase struct {
	*Condition
	Default bool
	Actions []Action
}

type ActionCondition struct {
	*ConditionCase

	Cases []ConditionCase
}

type SwitchCase struct {
	Compare *string
	Default bool
	Actions []Action
}

type ActionSwitch struct {
	Value    string
	Operator string
	Cases    []SwitchCase
}

type ActionLoop struct {
	Env   string
	Times int
	Seq   struct {
		From, To, Step int
	}
	Array []string
	Split struct {
		Value     string
		Separator string
	}

	Actions []Action
}

type ActionChmod struct {
	Path string
	Mode uint
}

type ActionChdir struct {
	Dir     string
	Actions []Action
}

type ActionCmd struct {
	Exec         string
	Stdin        string
	Stdout       string
	StdoutAppend bool
	Stderr       string
	StderrAppend bool
}

const (
	SilentFlagAllowError = "allowError"
	SilentFlagShowLog    = "showLog"
)

type ActionSilent struct {
	Flags   []string
	Actions []Action
}

type Action struct {
	Env       Env
	Cmd       ActionCmd
	Copy      ActionCopy
	Del       string
	Replace   ActionReplace
	Chmod     ActionChmod
	Chdir     ActionChdir
	Mkdir     string
	Template  string
	Condition ActionCondition
	Switch    ActionSwitch
	Loop      ActionLoop
	Silent    ActionSilent
}

type Task struct {
	Description string
	Depends     []string
	WorkDir     string

	Actions []Action
}

type Configuration struct {
	Envs      []Env
	Templates map[string][]Action
	Tasks     map[string]Task
}

func (c Configuration) searchTask(name string) (Task, bool) {
	task, ok := c.Tasks[name]
	return task, ok
}

func (c Configuration) searchTemplate(name string) ([]Action, bool) {
	tmpl, ok := c.Templates[name]
	return tmpl, ok
}

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

func stringRange(l int, args []string) (start, end int, err error) {
	var count int
	switch len(args) {
	case 0:
		start = 0
		count = l
	case 1:
		var err error
		start, err = strconv.Atoi(args[0])
		if err != nil {
			return 0, 0, fmt.Errorf("couldn't convert args to integer")
		}
		count = l
	case 2:
		var err1 error
		var err2 error
		start, err1 = strconv.Atoi(args[0])
		count, err2 = strconv.Atoi(args[0])
		if err1 != nil || err2 != nil {
			return 0, 0, fmt.Errorf("couldn't convert args to integer")
		}
	}
	if count < 0 {
		return 0, 0, fmt.Errorf("invalid args")
	}
	if start < 0 {
		last := -(l - 1)
		if start < last {
			start = last
		}
		if n := start - last + 1; count > n {
			count = n
		}
		start = l - start
	} else {
		last := l - 1
		if start > last {
			start = last
		}
		if n := last - start + 1; count > n {
			count = n
		}
	}
	end = start + count
	return
}

func stringReplace(val string, args []string, isRegexp bool) (string, error) {
	if len(args)%2 != 0 || len(args) <= 0 {
		return "", fmt.Errorf("invalid argument count")
	}
	if !isRegexp {
		if len(args) == 2 {
			val = strings.ReplaceAll(val, args[0], args[1])
		} else {
			r := strings.NewReplacer(args...)
			val = r.Replace(val)
		}
	} else {
		for i := 0; i < len(args); i += 2 {
			r, err := regexp.CompilePOSIX(args[i])
			if err != nil {
				return "", fmt.Errorf("compile regexp failed: %s, %w", args[i], err)
			}
			val = r.ReplaceAllString(val, args[i+1])
		}
	}
	return val, nil
}

func fileReplace(path string, args map[string]string, isRegexp bool) error {
	if len(args) == 0 {
		return nil
	}
	var fileContent []byte
	var err error
	fileContent, err = ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file failed: %w", err)
	}
	if !isRegexp {
		if len(args) == 1 {
			var old, new string
			for k, v := range args {
				old, new = k, v
			}
			fileContent = bytes.ReplaceAll(fileContent, []byte(old), []byte(new))
		} else {
			var oldnews []string
			for k, v := range args {
				oldnews = append(oldnews, k, v)
			}
			r := strings.NewReplacer(oldnews...)
			fileContent = []byte(r.Replace(string(fileContent)))
		}
	} else {
		for k, v := range args {
			r, err := regexp.CompilePOSIX(k)
			if err != nil {
				return fmt.Errorf("compile regexp failed: %s, %w", k, err)
			}
			fileContent = r.ReplaceAll(fileContent, []byte(v))
		}
	}
	err = ioutil.WriteFile(path, fileContent, 0644)
	if err != nil {
		return fmt.Errorf("write file failed: %w", err)
	}
	return nil
}

var expandFilters = map[string]func(val string, args []string) (string, error){
	"string.default": func(val string, args []string) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("invalid args")
		}
		if val == "" {
			return args[0], nil
		}
		return val, nil
	},
	"string.lower": func(val string, args []string) (string, error) {
		rs := []rune(val)
		start, end, err := stringRange(len(rs), args)
		if err != nil {
			return "", err
		}
		for i, v := 0, rs[start:end]; i < len(v); i++ {
			v[i] = unicode.ToLower(v[i])
		}
		return string(rs), nil
	},
	"string.upper": func(val string, args []string) (string, error) {
		rs := []rune(val)
		start, end, err := stringRange(len(rs), args)
		if err != nil {
			return "", err
		}
		for i, v := 0, rs[start:end]; i < len(v); i++ {
			v[i] = unicode.ToUpper(v[i])
		}
		return string(rs), nil
	},
	"string.slice": func(val string, args []string) (string, error) {
		rs := []rune(val)
		start, end, err := stringRange(len(rs), args)
		if err != nil {
			return "", err
		}
		return string(rs[start : start+end]), nil
	},
	"string.replace": func(val string, args []string) (string, error) {
		return stringReplace(val, args, false)
	},
	"string.regexpReplace": func(val string, args []string) (string, error) {
		return stringReplace(val, args, true)
	},
	"file.match": func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		matched, err := filepath.Glob(val)
		if err != nil {
			return "", fmt.Errorf("invalid file pattern: %s, %w", val, err)
		}
		return strings.Join(matched, " "), nil
	},
	"file.abspath": func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		abspath, err := filepath.Abs(val)
		if err != nil {
			return "", fmt.Errorf("get absolute path failed: %s, %w", val, err)
		}
		return abspath, nil
	},
	"file.dirname": func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return filepath.Dir(val), nil
	},
	"file.basename": func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return filepath.Base(val), nil
	},
}
