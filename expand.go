package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cosiner/argv"
)

type ExpandEnvs struct {
	envs map[string]string
}

func newVars() *ExpandEnvs {
	vars := &ExpandEnvs{
		envs: make(map[string]string),
	}

	return vars
}

func (e *ExpandEnvs) add(log logger, k, v string) {
	err := e.expandStrings(&v)
	if err != nil {
		log.fatalln(err)
	}
	log.debugln("env add:", k, v)
	e.envs[k] = v
}

func (e *ExpandEnvs) parseEnvs(log indentLogger, envs []Env) {
	for _, env := range envs {
		if env.Cmd != "" {
			output := runCommand(log, e, env.Cmd, true, commandFds{})
			if env.Name != "" {
				e.add(log, env.Name, strings.TrimSpace(output))
				continue
			}

			kvs := make(map[string]string)
			err := json.NewDecoder(strings.NewReader(output)).Decode(&kvs)
			if err == nil {
				for k, v := range kvs {
					e.add(log, k, v)
				}
				continue
			}
			lines := strings.Split(output, "\n")
			e.parseStrings(log, lines)
		} else if env.Value != "" {
			if env.Name == "" {
				e.parseStrings(log, []string{env.Value})
			} else {
				e.add(log, env.Name, env.Value)
			}
		} else if env.Name != "" {
			e.envs[env.Name] = ""
		}
	}
}

func (e *ExpandEnvs) parseStrings(log logger, items []string) {
	for _, item := range items {
		for _, env := range strings.Split(item, ";") {
			k, v := stringPartSplitAndTrim(env, "=")
			if k == "" || v == "" {
				continue
			}
			if uv, err := strconv.Unquote(v); err == nil {
				v = uv
			}

			e.add(log, k, v)
		}
	}
}

func (e *ExpandEnvs) formatEnvs() []string {
	var items []string
	for k, v := range e.envs {
		items = append(items, k+"="+v)
	}
	return items
}

func (e *ExpandEnvs) Exist(k string) bool {
	_, has := e.envs[k]
	return has
}

func (e *ExpandEnvs) expandStrings(s ...*string) error {
	for _, s := range s {
		v, err := e.expandString(*s)
		if err != nil {
			return fmt.Errorf("expand string failed: %s, %w", *s, err)
		}
		*s = v
	}
	return nil
}

func (e *ExpandEnvs) trimSpaceIfUnquoted(s string) string {
	us, err := strconv.Unquote(s)
	if err == nil && us != s {
		return us
	}
	return strings.TrimSpace(s)
}

func (e *ExpandEnvs) get(name string, filters []string) (string, error) {
	var val string
	us, err := strconv.Unquote(name)
	if err == nil && us != name {
		val = us
	} else {
		val = e.envs[name]
	}

	for _, filter := range filters {
		argv, err := argv.Argv(filter, nil, e.expandString)
		if err != nil {
			return "", fmt.Errorf("invalid expand filter: %s, %w", filter, err)
		}
		if len(argv) != 1 || len(argv[0]) == 0 {
			return "", fmt.Errorf("invalid expand filter syntax: %s", filter)
		}
		filterFunc, has := expandFilters[argv[0][0]]
		if !has {
			return "", fmt.Errorf("unrecognized expand filter: %s", filter)
		}
		val, err = filterFunc(val, argv[0][1:])
		if err != nil {
			return "", fmt.Errorf("execute expand filter failed: %s, %w", filter, err)
		}
	}
	return val, nil
}

func (e *ExpandEnvs) expandString(s string) (string, error) {
	const (
		statePlain = iota
		stateVar
		stateName
		stateBlockName
	)
	var (
		state = statePlain

		buf     []rune
		nameBuf []rune

		err error
	)
	resolveVar := func() []rune {
		name := string(nameBuf)

		var filters []string
		if strings.Index(name, "|") >= 0 {
			secs := stringSplitAndTrim(name, "|")
			name = secs[0]
			filters = secs[1:]
		}
		var v string
		v, err = e.get(name, filters)
		return []rune(v)
	}
	rs := []rune(s)
	l := len(rs)
	for i := 0; i < l; i++ {
		switch state {
		case statePlain:
			switch rs[i] {
			case '$':
				state = stateVar
				nameBuf = nameBuf[:0]
			case '\\':
				if i < l-1 {
					i++
					buf = append(buf, rs[i])
				}
			default:
				buf = append(buf, rs[i])
			}
		case stateVar:
			switch {
			case rs[i] == '{':
				state = stateBlockName
			case isAlphaNum(rs[i]):
				state = stateName
				nameBuf = append(nameBuf, rs[i])
			default:
				state = statePlain
				i--
			}
		case stateName:
			switch {
			case isAlphaNum(rs[i]):
				nameBuf = append(nameBuf, rs[i])
			default:
				buf = append(buf, resolveVar()...)
				if err != nil {
					return "", err
				}
				state = statePlain
				i--
			}
		case stateBlockName:
			switch rs[i] {
			case '}':
				buf = append(buf, resolveVar()...)
				if err != nil {
					return "", err
				}
				state = statePlain
			default:
				nameBuf = append(nameBuf, rs[i])
			}
		}
	}
	switch state {
	case statePlain:
	case stateVar:
	case stateName:
		if len(nameBuf) > 0 {
			buf = append(buf, resolveVar()...)
			if err != nil {
				return "", err
			}
		}
	case stateBlockName:
		if len(nameBuf) > 0 {
			buf = append(buf, nameBuf...)
		}
	}
	return string(buf), nil
}

// isAlphaNum reports whether the byte is an ASCII letter, number, or underscore
func isAlphaNum(c rune) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
