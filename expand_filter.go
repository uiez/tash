package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-zglob"
	"github.com/zhuah/tash/syntax"
)

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

var expandFilters = map[string]func(val string, args []string, envs *ExpandEnvs) (string, error){}

func init() {
	expandFilters[syntax.Ef_string_default] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("invalid args")
		}
		if val == "" {
			return args[0], nil
		}
		return val, nil
	}
	expandFilters[syntax.Ef_string_trimSpace] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return strings.TrimSpace(val), nil
	}
	expandFilters[syntax.Ef_string_trimPrefix] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("args invalid")
		}
		return strings.TrimPrefix(val, args[1]), nil
	}
	expandFilters[syntax.Ef_string_trimSuffix] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("args invalid")
		}
		return strings.TrimSuffix(val, args[1]), nil
	}
	expandFilters[syntax.Ef_string_lower] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		rs := []rune(val)
		start, end, err := stringRange(len(rs), args)
		if err != nil {
			return "", err
		}
		for i, v := 0, rs[start:end]; i < len(v); i++ {
			v[i] = unicode.ToLower(v[i])
		}
		return string(rs), nil
	}
	expandFilters[syntax.Ef_string_upper] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		rs := []rune(val)
		start, end, err := stringRange(len(rs), args)
		if err != nil {
			return "", err
		}
		for i, v := 0, rs[start:end]; i < len(v); i++ {
			v[i] = unicode.ToUpper(v[i])
		}
		return string(rs), nil
	}
	expandFilters[syntax.Ef_string_slice] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		rs := []rune(val)
		start, end, err := stringRange(len(rs), args)
		if err != nil {
			return "", err
		}
		return string(rs[start : start+end]), nil
	}
	expandFilters[syntax.Ef_string_at] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		var sep string
		var index string
		switch len(args) {
		case 1:
			sep = " "
			index = args[0]
		case 2:
			sep = args[0]
			index = args[1]
		default:
			return "", fmt.Errorf("args invalid")
		}

		secs := stringSplitAndTrimFilterSpace(val, sep)
		start, _, err := stringRange(len(secs), []string{index})
		if err != nil {
			return "", err
		}
		return secs[start], nil
	}
	expandFilters[syntax.Ef_string_replace] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return stringReplace(val, args, false)
	}
	expandFilters[syntax.Ef_string_regexpReplace] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return stringReplace(val, args, true)
	}
	expandFilters[syntax.Ef_string_sort] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		var sep string
		switch len(args) {
		case 0:
		case 1:
			sep = args[1]
		default:
			return "", fmt.Errorf("args invalid")
		}
		if sep == "" {
			sep = " "
		}
		secs := stringSplitAndTrimFilterSpace(val, sep)
		sort.Strings(secs)
		return strings.Join(secs, sep), nil
	}
	expandFilters[syntax.Ef_file_glob] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		var sep string
		switch len(args) {
		case 0:
		case 1:
			sep = args[1]
		default:
			return "", fmt.Errorf("args invalid")
		}
		if sep == "" {
			sep = " "
		}
		matched, err := zglob.Glob(val)
		if err != nil {
			return "", fmt.Errorf("invalid file pattern: %s, %w", val, err)
		}
		matched = sliceToSlash(matched)
		return strings.Join(matched, sep), nil
	}
	expandFilters[syntax.Ef_file_abspath] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		abspath, err := filepath.Abs(val)
		if err != nil {
			return "", fmt.Errorf("get absolute path failed: %s, %w", val, err)
		}
		return stringToSlash(abspath), nil
	}
	expandFilters[syntax.Ef_file_dirname] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return stringToSlash(filepath.Dir(val)), nil
	}
	expandFilters[syntax.Ef_file_basename] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return filepath.Base(val), nil
	}
	expandFilters[syntax.Ef_file_ext] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return filepath.Ext(val), nil
	}
	expandFilters[syntax.Ef_file_noext] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return strings.TrimSuffix(val, filepath.Ext(val)), nil
	}
	expandFilters[syntax.Ef_file_toSlash] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return filepath.ToSlash(val), nil
	}
	expandFilters[syntax.Ef_file_fromSlash] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return filepath.FromSlash(val), nil
	}
	expandFilters[syntax.Ef_file_content] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		content, err := ioutil.ReadFile(val)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}
	expandFilters[syntax.Ef_date_now] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		switch len(args) {
		case 0:
			return strconv.FormatInt(time.Now().Unix(), 10), nil
		case 1:
			return time.Now().Format(args[1]), nil
		default:
			return "", fmt.Errorf("args invalid")
		}
	}
	expandFilters[syntax.Ef_date_format] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("args invalid")
		}
		timestamp, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return "", err
		}
		t := time.Unix(timestamp, 0).In(time.Local)
		return t.Format(args[1]), nil
	}
	expandFilters[syntax.Ef_cmd_output] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return getCmdOutput(envs, val)
	}
}
