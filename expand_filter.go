package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
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

func parseRange(l int, args []string) (start, end int, err error) {
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
		start = l + start
	}
	if start < 0 {
		start = 0
	} else if start > l {
		start = l
	}
	if n := l - start; count > n {
		count = n
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
		start, end, err := parseRange(len(rs), args)
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
		start, end, err := parseRange(len(rs), args)
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
		start, end, err := parseRange(len(rs), args)
		if err != nil {
			return "", err
		}
		return string(rs[start : start+end]), nil
	}
	expandFilters[syntax.Ef_string_replace] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return stringReplace(val, args, false)
	}
	expandFilters[syntax.Ef_string_regexpReplace] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return stringReplace(val, args, true)
	}

	withArray := func(val string, args []string, fn func(arr []string) ([]string, error)) (string, error) {
		var sep string
		switch len(args) {
		case 0:
		case 1:
			sep = args[1]
		default:
			return "", fmt.Errorf("args invalid")
		}
		if sep == "" {
			sep = syntax.DefaultArraySeparator
		}
		arr := stringSplitAndTrimFilterSpace(val, sep)
		arr, err := fn(arr)
		if err != nil {
			return "", err
		}
		return strings.Join(arr, sep), nil
	}
	expandFilters[syntax.Ef_array_sort] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return withArray(val, args, func(arr []string) ([]string, error) {
			sort.Strings(arr)
			return arr, nil
		})
	}
	expandFilters[syntax.Ef_array_numSort] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return withArray(val, args, func(arr []string) ([]string, error) {
			nums := make([]int64, len(arr))
			for i := range arr {
				if arr[i] != "" {
					n, err := strconv.ParseInt(arr[i], 10, 64)
					if err != nil {
						return nil, fmt.Errorf("couldn't convert to number: '%s'", arr[i])
					}
					nums[i] = n
				}
			}
			sort.Slice(nums, func(i, j int) bool {
				return nums[i] < nums[j]
			})
			for i := range arr {
				arr[i] = strconv.FormatInt(nums[i], 10)
			}
			return arr, nil
		})
	}
	expandFilters[syntax.Ef_array_reverse] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return withArray(val, args, func(arr []string) ([]string, error) {
			l := len(arr)
			for i := 0; i < l/2; i++ {
				arr[i], arr[l-1-i] = arr[l-1-i], arr[i]
			}
			return arr, nil
		})
	}

	arrayAt := func(val string, args []string, isIndex, mustExist bool) (string, error) {
		var sep string
		var index string
		switch len(args) {
		case 1:
			sep = syntax.DefaultArraySeparator
			index = args[0]
		case 2:
			sep = args[0]
			index = args[1]
		default:
			return "", fmt.Errorf("args invalid")
		}

		arr := stringSplitAndTrimFilterSpace(val, sep)
		eleIdx, err := strconv.Atoi(index)
		if err != nil {
			return "", fmt.Errorf("convert element index to number failed: '%s'", index)
		}
		if !isIndex {
			if eleIdx < 0 {
				eleIdx = len(arr) + eleIdx
			}
		}

		valid := eleIdx >= 0 && eleIdx < len(arr)
		if valid {
			return arr[eleIdx], nil
		}
		if !mustExist {
			return "", nil
		}
		return "", fmt.Errorf("array element at given index not exist")
	}
	expandFilters[syntax.Ef_array_get] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return arrayAt(val, args, false, false)
	}
	expandFilters[syntax.Ef_array_mustGet] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return arrayAt(val, args, false, true)
	}
	expandFilters[syntax.Ef_array_at] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return arrayAt(val, args, true, false)
	}
	expandFilters[syntax.Ef_array_mustAt] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return arrayAt(val, args, true, true)
	}
	expandFilters[syntax.Ef_array_slice] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		var sep string
		var rangeArgs []string
		switch len(args) {
		case 1, 2:
			sep = syntax.DefaultArraySeparator
			rangeArgs = args
		case 3:
			sep = args[0]
			rangeArgs = args[1:]
		default:
			return "", fmt.Errorf("args invalid")
		}
		arr := stringSplitAndTrimFilterSpace(val, sep)
		start, end, err := parseRange(len(arr), rangeArgs)
		if err != nil {
			return "", err
		}
		arr = arr[start:end]
		return strings.Join(arr, sep), nil
	}
	expandFilters[syntax.Ef_array_separator] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		var oldSep string
		var newSep string
		switch len(args) {
		case 1:
			oldSep = syntax.DefaultArraySeparator
			newSep = args[0]
		case 2:
			oldSep = args[0]
			newSep = args[1]
		default:
			return "", fmt.Errorf("args invalid")
		}

		arr := stringSplitAndTrimFilterSpace(val, oldSep)
		return strings.Join(arr, newSep), nil
	}
	expandFilters[syntax.Ef_array_filter] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		var compare *string
		var sep string
		switch len(args) {
		case 1:
		case 2:
			compare = &args[1]
		case 3:
			compare = &args[1]
			sep = args[2]
		default:
			return "", fmt.Errorf("args invalid")
		}
		operator := args[0]
		if sep == "" {
			sep = syntax.DefaultArraySeparator
		}
		arr := stringSplitAndTrimFilterSpace(val, sep)
		var end int
		for i, s := range arr {
			ok, err := checkCondition(envs, s, operator, compare)
			if err != nil {
				return "", err
			}
			if ok {
				if end != i {
					arr[end] = arr[i]
				}
				end++
			}
		}
		arr = arr[:end]
		return strings.Join(arr, sep), nil
	}
	arrayIndex := func(val string, args []string, mustExist bool) (string, error) {
		var (
			sep string
			ele string
		)
		switch len(args) {
		case 1:
			ele = args[0]
		case 2:
			sep = args[0]
			ele = args[1]
		default:
			return "", fmt.Errorf("invalid args")
		}
		if sep == "" {
			sep = syntax.DefaultArraySeparator
		}
		arr := stringSplitAndTrimFilterSpace(val, sep)
		for i := range arr {
			if arr[i] == ele {
				return strconv.Itoa(i), nil
			}
		}
		if mustExist {
			return "", fmt.Errorf("element not found in array")
		}
		return "-1", nil
	}
	expandFilters[syntax.Ef_array_index] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return arrayIndex(val, args, false)
	}
	expandFilters[syntax.Ef_array_mustIndex] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		return arrayIndex(val, args, true)
	}

	parseMap := func(val string, sepArgs []string) (map[string]string, string, error) {
		var sep string
		switch len(sepArgs) {
		case 0:
		case 1:
			sep = sepArgs[0]
		default:
			return nil, "", fmt.Errorf("args invalid")
		}
		if sep == "" {
			sep = syntax.DefaultArraySeparator
		}
		arr := stringSplitAndTrimFilterSpace(val, sep)
		if len(arr)%2 != 0 {
			return nil, "", fmt.Errorf("map key/value is not paired")
		}
		if len(arr) == 0 {
			return nil, "", nil
		}
		m := make(map[string]string)
		for i := 0; i < len(arr); i += 2 {
			m[arr[i]] = arr[i+1]
		}
		return m, sep, nil
	}
	expandFilters[syntax.Ef_map_get] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		if len(args) == 0 {
			return "", fmt.Errorf("args invalid")
		}
		m, _, err := parseMap(val, args[1:])
		if err != nil {
			return "", err
		}
		return m[args[0]], nil
	}
	expandFilters[syntax.Ef_map_keys] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		m, sep, err := parseMap(val, args)
		if err != nil {
			return "", err
		}
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return strings.Join(keys, sep), nil
	}
	expandFilters[syntax.Ef_map_values] = func(val string, args []string, envs *ExpandEnvs) (string, error) {
		m, sep, err := parseMap(val, args)
		if err != nil {
			return "", err
		}
		var vals []string
		for k := range m {
			vals = append(vals, k)
		}
		sort.Strings(vals)
		for i, k := range vals {
			vals[i] = m[k]
		}
		return strings.Join(vals, sep), nil
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
			sep = syntax.DefaultArraySeparator
		}
		matched, err := zglob.Glob(val)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
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
		if err != nil && !os.IsNotExist(err) {
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
		var workingDir string
		switch len(args) {
		case 0:
		case 1:
			err := envs.expandStringPtrs(&args[0])
			if err != nil {
				return "", err
			}
			workingDir = args[0]
		default:
			return "", fmt.Errorf("args invalid")
		}
		return getCmdStringOutput(envs, val, workingDir)
	}
}
