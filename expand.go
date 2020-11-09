package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/cosiner/argv"
	"github.com/uiez/tash/syntax"
)

type ExpandEnvs struct {
	envs map[string]string
}

func newExpandEnvs() *ExpandEnvs {
	vars := &ExpandEnvs{
		envs: make(map[string]string),
	}

	return vars
}
func (e *ExpandEnvs) copy() *ExpandEnvs {
	ne := ExpandEnvs{
		envs: make(map[string]string),
	}
	for k, v := range e.envs {
		ne.envs[k] = v
	}
	return &ne
}

func (e *ExpandEnvs) remove(k string) {
	delete(e.envs, k)
}

func (e *ExpandEnvs) get(k string) (string, bool) {
	v, has := e.envs[k]
	return v, has
}

func (e *ExpandEnvs) set(k, v string) {
	e.envs[k] = v
	if k == "PATH" {
		os.Setenv(k, v)
	}
}
func (e *ExpandEnvs) addAndExpand(log logger, k, v string, expand bool) {
	if expand {
		err := e.expandStringPtrs(&v)
		if err != nil {
			log.fatalln(err)
		}
	}

	log.debugln("env add:", k, v)
	e.set(k, v)
}

func (e *ExpandEnvs) parseEnv(log indentLogger, envs syntax.EnvList) {
	for _, env := range envs.Envs() {
		blocks := splitBlocks(env)
		e.parsePairs(log, blocks, true)
	}
}

func (e *ExpandEnvs) parsePairs(log logger, items []string, expand bool) {
	for _, item := range items {
		if item == "" {
			continue
		}
		k, v := stringSplitAndTrimToPair(item, "=")
		if k == "" || v == "" {
			continue
		}
		v = stringUnquote(v)

		e.addAndExpand(log, k, v, expand)
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

func (e *ExpandEnvs) expandStringSlice(s []string) error {
	for i := range s {
		v, err := e.expandString(s[i])
		if err != nil {
			return fmt.Errorf("expand string failed: %s, %w", s[i], err)
		}
		s[i] = v
	}
	return nil
}

func (e *ExpandEnvs) expandStringPtrs(s ...*string) error {
	for _, s := range s {
		if s == nil {
			continue
		}
		v, err := e.expandString(*s)
		if err != nil {
			return fmt.Errorf("expand string failed: %s, %w", *s, err)
		}
		*s = v
	}
	return nil
}

func (e *ExpandEnvs) trimSpaceIfUnquoted(s string) string {
	us := stringUnquote(s)
	if us != s {
		return us
	}
	return strings.TrimSpace(s)
}

func (e *ExpandEnvs) lookupAndFilter(name string, filters []string) (string, error) {
	var val string
	if us := stringUnquote(name); us != name {
		val = us
	} else {
		val = e.envs[name]
	}

	for _, filter := range filters {
		originFilter := filter
		filter = stringUnquote(filter)
		filter, err := e.expandString(filter)
		if err != nil {
			return "", fmt.Errorf("invalid expand filter: %s, %w", originFilter, err)
		}

		argv, err := argv.Argv(filter, nil, func(s string) (string, error) {
			return s, nil
		})
		if err != nil {
			return "", fmt.Errorf("invalid expand filter: %s, %w", originFilter, err)
		}
		args := argv[0]
		if len(argv) != 1 || len(args) == 0 {
			return "", fmt.Errorf("invalid expand filter syntax: %s, %v", originFilter, argv)
		}
		filterFunc, has := expandFilters[args[0]]
		if !has && syntax.IsValidOP(args[0]) {
			args = append([]string{syntax.Ef_condition_check}, args...)
			filterFunc, has = expandFilters[args[0]]
		}
		if !has {
			return "", fmt.Errorf("unrecognized expand filter: %s", originFilter)
		}
		val, err = filterFunc(val, args[1:], e)
		if err != nil {
			return "", fmt.Errorf("execute expand filter failed: %s, %w", originFilter, err)
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

		buf              []rune
		nameBuf          []rune
		nameBlockFilters []int
		nameBlockDepth   int

		err error
	)
	resolveVar := func() []rune {
		var name string
		var filters []string
		if len(nameBlockFilters) > 0 {
			for i := range nameBlockFilters {
				if i == 0 {
					name = string(nameBuf[:nameBlockFilters[i]])
				} else {
					filters = append(filters, string(nameBuf[nameBlockFilters[i-1]+1:nameBlockFilters[i]]))
				}
			}
			filters = append(filters, string(nameBuf[nameBlockFilters[len(nameBlockFilters)-1]+1:]))
		} else {
			name = string(nameBuf)
		}
		name = strings.TrimSpace(name)
		for i := range filters {
			filters[i] = strings.TrimSpace(filters[i])
		}
		var v string
		v, err = e.lookupAndFilter(name, filters)
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
				nameBlockFilters = nameBlockFilters[:0]
				nameBlockDepth = 0
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
			case '\\':
				nameBuf = append(nameBuf, rs[i])
				if i < l-1 {
					i++
					nameBuf = append(nameBuf, rs[i])
				}
			case '{':
				nameBuf = append(nameBuf, rs[i])
				nameBlockDepth++
			case '|':
				nameBuf = append(nameBuf, rs[i])
				if nameBlockDepth == 0 {
					nameBlockFilters = append(nameBlockFilters, len(nameBuf)-1)
				}
			case '}':
				if nameBlockDepth > 0 {
					nameBuf = append(nameBuf, rs[i])
					nameBlockDepth--
				} else {
					buf = append(buf, resolveVar()...)
					if err != nil {
						return "", err
					}
					state = statePlain
				}
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
