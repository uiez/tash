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
	secs := strings.SplitN(s, sep, 2)
	return stringAtAndTrim(secs, 0), stringAtAndTrim(secs, 1)
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
	copyFile := func(dst, src string) error {
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
	case ResourceHashAlgSha1:
		hashCreator = sha1.New
	case ResourceHashAlgMD5:
		hashCreator = md5.New
	case ResourceHashAlgSha256:
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

func runCommand(log indentLogger, vars *ExpandEnvs, cmd string, needsOutput bool, fds commandFds) string {
	osEnvs := vars.formatEnvs()
	sections, err := argv.Argv(
		cmd,
		func(cmd string) (string, error) {
			output := runCommand(log.addIndent(), vars, cmd, true, commandFds{})
			return output, nil
		},
		func(s string) (string, error) {
			return vars.expandString(s)
		},
	)
	if err != nil {
		log.fatalln("parse command failed:", cmd, err)
		return ""
	}
	cmds, err := argv.Cmds(sections...)
	if err != nil {
		log.fatalln("create command failed:", cmd, err)
		return ""
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
		log.fatalln("run command failed:", cmd, err)
		return ""
	}
	if needsOutput {
		return fds.Stdout.(*bytes.Buffer).String()
	}
	return ""
}

type conditionContext struct {
	log           logger
	envs          *ExpandEnvs
	valueOrigin   string
	compareOrigin string
}

func checkCondition(ctx *conditionContext, value, operator, compare string) bool {
	fixAlias := func(o *string) {
		if a, has := operatorAlias[*o]; has {
			*o = a
		}
	}
	if operator == "" {
		if ctx.compareOrigin == "" {
			operator = op_bool_true
		} else {
			operator = op_string_equal
		}
	}
	var (
		not bool
	)
	if idx := strings.Index(operator, " "); idx > 0 {
		n, o := stringSplitAndTrimToPair(operator, " ")
		fixAlias(&n)
		if n == op_bool_not {
			not = true
			operator = o
		}
	}
	fixAlias(&operator)
	var ok bool
	switch operator {
	case op_string_regexp:
		r, err := regexp.CompilePOSIX(compare)
		if err != nil {
			ctx.log.fatalln("compile regexp failed:", compare, err)
		}
		ok = r.MatchString(value)

	case op_string_greaterThan:
		ok = value > compare
	case op_string_greaterThanOrEqual:
		ok = value >= compare
	case op_string_equal:
		ok = value == compare
	case op_string_notEqual:
		ok = value != compare
	case op_string_lessThanOrEqual:
		ok = value <= compare
	case op_string_lessThan:
		ok = value < compare

	case op_number_greaterThan, op_number_greaterThanOrEqual, op_number_equal, op_number_notEqual, op_number_lessThanOrEqual, op_number_lessThan:
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
			ctx.log.fatalln("convert values to float number failed:", value, compare)
		}
		switch operator {
		case op_number_greaterThan:
			ok = v1 > v2
		case op_number_greaterThanOrEqual:
			ok = v1 >= v2
		case op_number_equal:
			ok = v1 == v2
		case op_number_notEqual:
			ok = v1 != v2
		case op_number_lessThanOrEqual:
			ok = v1 <= v2
		case op_number_lessThan:
			ok = v1 < v2
		}
	case op_file_newerThan, op_file_olderThan:
		s1, e1 := os.Stat(value)
		s2, e2 := os.Stat(compare)
		if e1 != nil || e2 != nil {
			ctx.log.fatalln("access files failed:", e1, e2)
		}
		switch operator {
		case op_file_newerThan:
			ok = s1.ModTime().After(s2.ModTime())
		case op_file_olderThan:
			ok = s1.ModTime().Before(s2.ModTime())
		}
	//case "-ef":
	default:
		if compare != "" {
			ctx.log.fatalln("operator doesn't needs compare field:", operator)
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
		case op_string_notEmpty:
			ok = value != ""
		case op_string_empty:
			ok = value == ""
		case op_bool_true, op_bool_not:
			switch strings.ToLower(value) {
			case "true", "yes", "1":
				ok = true
			case "", "false", "no", "0":
				ok = false
			default:
				ctx.log.fatalln("invalid boolean value:", ctx.valueOrigin, value)
			}
			if operator == op_bool_not {
				ok = !ok
			}
		case op_env_defined:
			ok = ctx.envs.Exist(value)
		case op_file_exist:
			ok = checkFileStat(nil)
		case op_file_blockDevice:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0
			})
		case op_file_charDevice:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeDevice != 0 && mode&os.ModeCharDevice != 0
			})
		case op_file_dir:
			ok = checkFileStat(func(stat os.FileInfo) bool {
				return stat.IsDir()
			})
		case op_file_regular:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode.IsRegular()
			})
		case op_file_setgid:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSetgid != 0
			})
		//case "-G":
		case op_file_symlink:
			ok = checkFileLstatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSymlink != 0
			})
		case op_file_sticky:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSticky != 0
			})
		//case "-N":
		//case "-O":
		case op_file_namedPipe:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeNamedPipe != 0
			})
		//case "-r":

		case op_file_notEmpty:
			ok = checkFileStat(func(stat os.FileInfo) bool {
				return stat.Size() > 0
			})
		case op_file_socket:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSocket != 0
			})
		//case "-t":
		case op_file_setuid:
			ok = checkFileStatMode(func(mode os.FileMode) bool {
				return mode&os.ModeSetuid != 0
			})
		//case "-w":
		//case "-x":
		default:
			ctx.log.fatalln("invalid condition operator:", operator)
		}
	}

	if not {
		return !ok
	}
	return ok
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
