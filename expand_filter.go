package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
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

var expandFilters = map[string]func(val string, args []string) (string, error){
	ef_stringDefault: func(val string, args []string) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("invalid args")
		}
		if val == "" {
			return args[0], nil
		}
		return val, nil
	},
	ef_stringTrimSpace: func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return strings.TrimSpace(val), nil
	},
	ef_stringLower: func(val string, args []string) (string, error) {
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
	ef_stringUpper: func(val string, args []string) (string, error) {
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
	ef_stringSlice: func(val string, args []string) (string, error) {
		rs := []rune(val)
		start, end, err := stringRange(len(rs), args)
		if err != nil {
			return "", err
		}
		return string(rs[start : start+end]), nil
	},
	ef_stringReplace: func(val string, args []string) (string, error) {
		return stringReplace(val, args, false)
	},
	ef_stringRegexpReplace: func(val string, args []string) (string, error) {
		return stringReplace(val, args, true)
	},
	ef_fileGlob: func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		matched, err := filepath.Glob(val)
		if err != nil {
			return "", fmt.Errorf("invalid file pattern: %s, %w", val, err)
		}
		matched = sliceToSlash(matched)
		return strings.Join(matched, " "), nil
	},
	ef_fileAbspath: func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		abspath, err := filepath.Abs(val)
		if err != nil {
			return "", fmt.Errorf("get absolute path failed: %s, %w", val, err)
		}
		return stringToSlash(abspath), nil
	},
	ef_fileDirname: func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return stringToSlash(filepath.Dir(val)), nil
	},
	ef_fileBasename: func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return filepath.Base(val), nil
	},
	ef_fileToSlash: func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return filepath.ToSlash(val), nil
	},
	ef_fileFromSlash: func(val string, args []string) (string, error) {
		if len(args) != 0 {
			return "", fmt.Errorf("args is not needed")
		}
		return filepath.FromSlash(val), nil
	},
}
