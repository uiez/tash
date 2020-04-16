package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cosiner/argv"
	"github.com/zhuah/tash/syntax"
)

type logger interface {
	debugln(v ...interface{})
	infoln(v ...interface{})
	warnln(v ...interface{})
	fatalln(v ...interface{})
}

func stringAtAndTrim(s []string, i int) string {
	if i < len(s) {
		return strings.TrimSpace(s[i])
	}
	return ""
}

func stringSplitAndTrimFilterSpace(s, sep string) []string {
	secs := stringSplitAndTrim(s, sep)
	var end int
	for i := range secs {
		if secs[i] != "" {
			if i != end {
				secs[end] = secs[i]
			}
			end++
		}
	}
	return secs[:end]
}

func stringSplitAndTrim(s, sep string) []string {
	secs := strings.Split(s, sep)
	for i := range secs {
		secs[i] = strings.TrimSpace(secs[i])
	}
	return secs
}

func stringSplitAndTrimToPair(s, sep string) (s1, s2 string) {
	var secs []string
	if sep == " " {
		secs = stringSplitAndTrimFilterSpace(s, sep)
	} else {
		secs = strings.SplitN(s, sep, 2)
	}
	return stringAtAndTrim(secs, 0), stringAtAndTrim(secs, 1)
}

func copyFile(dst, src string) error {
	srcFd, err := os.OpenFile(src, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	srcStat, err := srcFd.Stat()
	if err != nil {
		return err
	}
	defer srcFd.Close()
	dstFd, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer dstFd.Close()
	_, err = io.Copy(dstFd, srcFd)
	if err == nil {
		err = os.Chmod(dst, srcStat.Mode())
	}
	if err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}

func copyPath(dst, src string) error {
	stat, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("read source path status failed: %w", err)
	}
	err = os.RemoveAll(dst)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove dst path failed: %w", err)
	}
	err = os.MkdirAll(filepath.Dir(dst), 0755)
	if err != nil {
		return fmt.Errorf("create dst path dirs failed: %w", err)
	}
	if !stat.IsDir() {
		err = os.MkdirAll(filepath.Dir(dst), 0755)
		if err != nil {
			return fmt.Errorf("create dst parent directory tree failed: %w", err)
		}
		return copyFile(dst, src)
	}
	dirChmods := map[string]os.FileMode{}
	err = filepath.Walk(src, func(srcPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, srcPath)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			err = os.Mkdir(dstPath, 0755)
			if err != nil {
				return err
			}
			if info.Mode() != 0755 {
				dirChmods[dstPath] = info.Mode()
			}
			return nil
		}
		return copyFile(dstPath, srcPath)
	})
	if err != nil {
		return fmt.Errorf("copy path tree failed: %w", err)
	}
	for dir, mode := range dirChmods {
		err = os.Chmod(dir, mode)
		if err != nil {
			return fmt.Errorf("fix dir mod failed: %w", err)
		}
	}
	return nil
}

func checkHash(log logger, path string, alg, sig string, r io.Reader) bool {
	var hashCreator func() hash.Hash
	switch alg {
	case syntax.ResourceHashAlgSha1:
		hashCreator = sha1.New
	case syntax.ResourceHashAlgMD5:
		hashCreator = md5.New
	case syntax.ResourceHashAlgSha256:
		hashCreator = sha256.New
	}
	if hashCreator == nil || sig == "" {
		log.fatalln("invalid hash alg or sig:", path)
		return false
	}
	h := hashCreator()
	_, err := io.Copy(h, r)
	if err != nil {
		log.fatalln("check hash failed:", path, err)
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == strings.ToLower(sig)
}

func downloadFile(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("create download request failed: %w", err)
	}
	defer resp.Body.Close()

	fd, err := ioutil.TempFile("", "tash*")
	if err != nil {
		return "", fmt.Errorf("create tmp file failed: %w", err)
	}
	defer fd.Close()
	_, err = io.Copy(fd, resp.Body)
	if err != nil {
		return "", fmt.Errorf("download file failed: %w", err)
	}
	return fd.Name(), nil
}

type commandFds struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func runCommand(envs *ExpandEnvs, cmd string, needsOutput bool, fds commandFds) (string, error) {
	osEnvs := envs.formatEnvs()
	sections, err := argv.Argv(
		cmd,
		func(cmd string) (string, error) {
			return getCmdOutput(envs, cmd)
		},
		envs.expandString,
	)
	if err != nil {
		return "", fmt.Errorf("parse command string failed: %s", err)
	}
	cmds, err := argv.Cmds(sections...)
	if err != nil {
		return "", fmt.Errorf("build command failed: %s", err)
	}
	for i := range cmds {
		cmds[i].Env = osEnvs
	}
	if needsOutput {
		fds.Stdin = nil
		fds.Stdout = bytes.NewBuffer(nil)
		fds.Stderr = nil
	}
	err = argv.Pipe(fds.Stdin, fds.Stdout, fds.Stderr, cmds...)
	if err != nil {
		return "", fmt.Errorf("run command failed: %s", err)
	}
	if needsOutput {
		return fds.Stdout.(*bytes.Buffer).String(), nil
	}
	return "", nil
}

func getCmdOutput(envs *ExpandEnvs, cmd string) (string, error) {
	return runCommand(envs, cmd, true, commandFds{})
}

func checkCondition(envs *ExpandEnvs, value, operator string, compareField *string) (bool, error) {
	fixAlias := func(o *string) {
		if a, has := syntax.OperatorAlias[*o]; has {
			*o = a
		}
	}
	if operator == "" {
		if compareField != nil {
			operator = syntax.Op_bool_true
		} else {
			operator = syntax.Op_string_equal
		}
	}
	var (
		not bool
	)
	if idx := strings.Index(operator, " "); idx > 0 {
		n, o := stringSplitAndTrimToPair(operator, " ")
		fixAlias(&n)
		if n == syntax.Op_bool_not {
			not = true
			operator = o
		}
	}
	fixAlias(&operator)

	var compare string
	if compareField != nil {
		compare = *compareField
	}
	var ok bool
	switch operator {
	case syntax.Op_string_regexp:
		r, err := regexp.CompilePOSIX(compare)
		if err != nil {
			return false, fmt.Errorf("compile regexp failed: %s, %s", compare, err)
		}
		ok = r.MatchString(value)

	case syntax.Op_string_greaterThan:
		ok = value > compare
	case syntax.Op_string_greaterThanOrEqual:
		ok = value >= compare
	case syntax.Op_string_equal:
		ok = value == compare
	case syntax.Op_string_notEqual:
		ok = value != compare
	case syntax.Op_string_lessThanOrEqual:
		ok = value <= compare
	case syntax.Op_string_lessThan:
		ok = value < compare

	case syntax.Op_number_greaterThan, syntax.Op_number_greaterThanOrEqual, syntax.Op_number_equal, syntax.Op_number_notEqual, syntax.Op_number_lessThanOrEqual, syntax.Op_number_lessThan:
		parseInt := func(s string) (int64, error) {
			for prefix, base := range map[string]int{
				"0x": 16,
				"0o": 8,
				"0b": 2,
			} {
				if strings.HasPrefix(s, prefix) {
					return strconv.ParseInt(s, base, 64)
				}
			}
			return strconv.ParseInt(s, 10, 64)
		}
		var (
			v1, v2     int64
			err1, err2 error
		)
		if value != "" {
			v1, err1 = parseInt(value)
		}
		if v := compare; v != "" {
			v2, err2 = parseInt(v)
		}
		if err1 != nil || err2 != nil {
			return false, fmt.Errorf("convert values to float number failed: %s, %s", value, compare)
		}
		switch operator {
		case syntax.Op_number_greaterThan:
			ok = v1 > v2
		case syntax.Op_number_greaterThanOrEqual:
			ok = v1 >= v2
		case syntax.Op_number_equal:
			ok = v1 == v2
		case syntax.Op_number_notEqual:
			ok = v1 != v2
		case syntax.Op_number_lessThanOrEqual:
			ok = v1 <= v2
		case syntax.Op_number_lessThan:
			ok = v1 < v2
		}
	case syntax.Op_file_newerThan, syntax.Op_file_olderThan:
		s1, e1 := os.Stat(value)
		s2, e2 := os.Stat(compare)
		if e1 != nil || e2 != nil {
			return false, fmt.Errorf("access files failed: %s %s", e1, e2)
		}
		switch operator {
		case syntax.Op_file_newerThan:
			ok = s1.ModTime().After(s2.ModTime())
		case syntax.Op_file_olderThan:
			ok = s1.ModTime().Before(s2.ModTime())
		}
	//case "-ef":
	default:
		if compareField != nil {
			return false, fmt.Errorf("operator doesn't needs compare field: %s", operator)
		}

		checkFileStat := func(fn func(stat os.FileInfo) bool) bool {
			stat, err := os.Stat(value)
			return err == nil && (fn == nil || fn(stat))
		}
		checkFileStatMode := func(fn func(mode os.FileMode) bool) bool {
			return checkFileStat(func(stat os.FileInfo) bool {
				return fn(stat.Mode())
			})
		}
		checkFileLStat := func(fn func(stat os.FileInfo) bool) bool {
			stat, err := os.Lstat(value)
			return err == nil && (fn == nil || fn(stat))
		}
		checkFileLstatMode := func(fn func(mode os.FileMode) bool) bool {
			return checkFileLStat(func(stat os.FileInfo) bool {
				return fn(stat.Mode())
			})
		}
		switch operator {
		case syntax.Op_string_notEmpty:
			ok = value != ""
		case syntax.Op_string_empty:
			ok = value == ""
		case syntax.Op_bool_true, syntax.Op_bool_not:
			switch strings.ToLower(value) {
			case "true", "yes", "1":
				ok = true
			case "", "false", "no", "0":
				ok = false
			default:
				return false, fmt.Errorf("invalid boolean value: %s", value)
			}
			if operator == syntax.Op_bool_not {
				ok = !ok
			}
		case syntax.Op_env_defined:
			ok = envs.Exist(value)
		case syntax.Op_file_exist:
			ok = checkFileStat(nil)
		case syntax.Op_file_blockDevice:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0
			})
		case syntax.Op_file_charDevice:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeDevice != 0 && mode&os.ModeCharDevice != 0
			})
		case syntax.Op_file_dir:
			ok = checkFileStat(func(stat os.FileInfo) bool {
				return stat.IsDir()
			})
		case syntax.Op_file_regular:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode.IsRegular()
			})
		case syntax.Op_file_setgid:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSetgid != 0
			})
		//case "-G":
		case syntax.Op_file_symlink:
			ok = checkFileLstatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSymlink != 0
			})
		case syntax.Op_file_sticky:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSticky != 0
			})
		//case "-N":
		//case "-O":
		case syntax.Op_file_namedPipe:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeNamedPipe != 0
			})
		//case "-r":

		case syntax.Op_file_notEmpty:
			ok = checkFileStat(func(stat os.FileInfo) bool {
				return stat.Size() > 0
			})
		case syntax.Op_file_socket:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSocket != 0
			})
		//case "-t":
		case syntax.Op_file_setuid:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSetuid != 0
			})
		//case "-w":
		//case "-x":
		default:
			return false, fmt.Errorf("invalid condition operator: %s", operator)
		}
	}

	if not {
		return !ok, nil
	}
	return ok, nil
}

func fileReplacer(args []string, isRegexp bool) (func(path string) error, error) {
	if len(args) == 0 {
		return func(path string) error {
			return nil
		}, nil
	}
	withFileContent := func(fn func([]byte) []byte) func(path string) error {
		return func(path string) error {
			fd, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				return err
			}
			defer fd.Close()
			content, err := ioutil.ReadAll(fd)
			if err != nil {
				return err
			}
			content = fn(content)
			_, err = fd.Seek(0, io.SeekStart)
			if err == nil {
				err = fd.Truncate(0)
			}
			if err == nil {
				_, err = fd.Write(content)
			}
			return err
		}
	}
	if !isRegexp {
		if len(args) == 2 {
			o := []byte(args[0])
			n := []byte(args[1])
			return withFileContent(func(data []byte) []byte {
				return bytes.ReplaceAll(data, o, n)
			}), nil
		}
		r := strings.NewReplacer(args...)
		return withFileContent(func(data []byte) []byte {
			return []byte(r.Replace(string(data)))
		}), nil
	}

	type regPair struct {
		R       *regexp.Regexp
		Replace []byte
	}
	var regs []regPair
	for i := 0; i < len(args); i += 2 {
		r, err := regexp.CompilePOSIX(args[i])
		if err != nil {
			return nil, fmt.Errorf("compile regexp failed: %s, %w", args[i], err)
		}
		regs = append(regs, regPair{R: r, Replace: []byte(args[i+1])})
	}
	return withFileContent(func(data []byte) []byte {
		for _, p := range regs {
			data = p.R.ReplaceAll(data, p.Replace)
		}
		return data
	}), nil
}

func stringToSlash(s string) string {
	return filepath.ToSlash(s)
}

func ptrsToSlash(ptr ...*string) {
	for _, ptr := range ptr {
		*ptr = filepath.ToSlash(*ptr)
	}
}

func sliceToSlash(paths []string) []string {
	for i := range paths {
		paths[i] = filepath.ToSlash(paths[i])
	}
	return paths
}

func openFile(name string, append bool) (*os.File, error) {
	flags := os.O_WRONLY | os.O_CREATE
	if append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	err := os.MkdirAll(filepath.Dir(name), 0755)
	if err != nil {
		return nil, fmt.Errorf("create parent directories failed: %w", err)
	}
	return os.OpenFile(name, flags, 00644)
}
