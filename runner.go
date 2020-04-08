package main

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
)

type indentLogger struct {
	indent     string
	debug      bool
	hideLog    bool
	allowError bool
}

func (w indentLogger) addIndent() indentLogger {
	return indentLogger{
		indent: w.indent + "    ",
		debug:  w.debug,
	}
}

func (w indentLogger) addIndentIfDebug() indentLogger {
	if w.debug {
		return w.addIndent()
	}
	return w
}

func (w indentLogger) silent(hideLog, allowError bool) indentLogger {
	return indentLogger{
		indent:     w.indent,
		debug:      w.debug,
		hideLog:    hideLog,
		allowError: allowError,
	}
}

func (w indentLogger) print(fg color.Attribute, out io.Writer, v ...interface{}) {
	if !w.hideLog || w.debug {
		fmt := color.New(fg)
		_, _ = fmt.Fprint(out, w.indent)
		_, _ = fmt.Fprintln(out, v...)
	}
}

func (w indentLogger) fatalln(v ...interface{}) {
	w.print(color.FgHiRed, os.Stderr, v...)
	if !w.allowError {
		os.Exit(1)
	}
}

func (w indentLogger) infoln(v ...interface{}) {
	w.print(color.FgHiGreen, os.Stdout, v...)
}

func (w indentLogger) warnln(v ...interface{}) {
	w.print(color.FgHiYellow, os.Stdout, v...)
}

func (w indentLogger) debugln(v ...interface{}) {
	if w.debug {
		w.print(color.FgHiWhite, os.Stdout, v...)
	}
}

func newLogger(debug bool) indentLogger {
	return indentLogger{
		debug: debug,
	}
}

func listTasks(configs *Configuration, log indentLogger) {
	if len(configs.Tasks) == 0 {
		log.infoln("no tasks defined.")
	} else {
		log.infoln("available tasks:")
		var buf bytes.Buffer
		var names []string
		for name := range configs.Tasks {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			task := configs.Tasks[name]
			buf.WriteString("    - ")
			buf.WriteString(name)
			if len(task.Depends) > 0 {
				fmt.Fprintf(&buf, "(deps: %v)", task.Depends)
			}
			fmt.Fprintf(&buf, ": %s", task.Description)
			log.infoln(buf.String())
			buf.Reset()
		}
	}
}

func runTasks(configs *Configuration, names []string, log indentLogger) {
	currDir, err := os.Getwd()
	if err != nil {
		log.fatalln("get current directory failed:", err)
		return
	}

	r := runner{
		indentLogger: log,
		configs:      configs,
	}
	for i, name := range names {
		if i > 0 {
			r.infoln() // create new line
		}
		runned := make(map[string]int)
		r.runTaskWithDeps(runned, name, name, currDir)
	}
}

type runner struct {
	indentLogger
	configs *Configuration
}

func (r runner) log() indentLogger {
	return r.indentLogger
}

func (r runner) addIndent() runner {
	return runner{
		indentLogger: r.log().addIndent(),
		configs:      r.configs,
	}
}

func (r runner) addIndentIfDebug() runner {
	return runner{
		indentLogger: r.log().addIndentIfDebug(),
		configs:      r.configs,
	}
}

func (r runner) silent(hideLog, allowError bool) runner {
	return runner{
		indentLogger: r.log().silent(hideLog, allowError),
		configs:      r.configs,
	}
}

func (r runner) searchTask(name string) (Task, bool) {
	task, ok := r.configs.Tasks[name]
	return task, ok
}

func (r runner) searchTemplate(name string) ([]Action, bool) {
	tmpl, ok := r.configs.Templates[name]
	return tmpl, ok
}

func (r runner) runTask(name string, task Task, baseDir string) {
	workDir := stringToSlash(filepath.Join(baseDir, task.WorkDir))
	err := os.Chdir(workDir)
	if err != nil {
		r.fatalln("change working directory failed:", err)
		return
	}

	r.infoln("WorkDir:", workDir)
	envs := newExpandEnvs()
	envs.parsePairs(r.log(), os.Environ(), false)
	envs.add(r.log(), "WORKDIR", workDir, false)
	envs.add(r.log(), "HOST_OS", runtime.GOOS, false)
	envs.add(r.log(), "HOST_ARCH", runtime.GOARCH, false)
	envs.add(r.log(), "TASK_NAME", name, false)
	envs.parseEnvs(r.log(), r.configs.Envs)

	r.runActions(envs, task.Actions)
}

func (r runner) runTaskWithDeps(runned map[string]int, owner, name, baseDir string) {
	switch runned[name] {
	case 0:
	case 1:
		r.fatalln("task cycle dependency for:", owner)
	case 2:
		return
	}
	runned[name] = 1

	task, ok := r.searchTask(name)
	if !ok {
		r.fatalln("task not found:", name)
	}
	if len(task.Depends) > 0 {
		r.infoln("Task Deps:", name, task.Depends)
		for i, d := range task.Depends {
			if i > 0 {
				r.infoln()
			}
			r.addIndent().runTaskWithDeps(runned, owner, d, baseDir)
		}
	}
	r.infoln("Task:", name)
	r.addIndent().runTask(name, task, baseDir)

	runned[name] = 2
}

func (r runner) resourceNeedsSync(cpy ActionCopy) bool {
	info, err := os.Stat(cpy.DestPath)
	if err != nil {
		return true
	}
	if cpy.Hash.Sig == "" || info.IsDir() {
		return false
	}
	fd, err := os.OpenFile(cpy.DestPath, os.O_RDONLY, 0)
	if err != nil {
		return true
	}
	defer fd.Close()

	return checkHash(r.log(), cpy.DestPath, cpy.Hash.Alg, cpy.Hash.Sig, fd)
}

func (r runner) resourceIsValid(res ActionCopy, path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		r.fatalln("check resource stat failed:", err)
		return false
	}
	if res.Hash.Sig == "" || info.IsDir() {
		return true
	}
	fd, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		r.fatalln("open resource failed:", err)
		return false
	}
	defer fd.Close()

	return checkHash(r.log(), res.SourceUrl, res.Hash.Alg, res.Hash.Sig, fd)
}

func (r runner) runActionCopy(cpy ActionCopy, envs *ExpandEnvs) {
	if !r.resourceNeedsSync(cpy) {
		r.debugln("resource reuse.")
		return
	}
	var sourcePath string
	if strings.Contains(cpy.SourceUrl, ":/") {
		sourceUrl := cpy.SourceUrl
		ul, err := url.Parse(sourceUrl)
		if err != nil {
			r.fatalln("couldn't parse source url:", sourceUrl, err)
		}
		switch ul.Scheme {
		case "file":
			sourcePath = ul.Path
			if runtime.GOOS == "windows" {
				sourcePath = strings.TrimPrefix(sourcePath, "/")
			}
		case "http", "https":
			path, err := downloadFile(cpy.SourceUrl)
			if err != nil {
				r.fatalln("download file failed:", cpy.SourceUrl, err)
				return
			}
			sourcePath = path
		default:
			r.fatalln("unsupported source url schema:", ul.Scheme)
		}
	} else {
		sourcePath = cpy.SourceUrl
	}
	if !r.resourceIsValid(cpy, sourcePath) {
		r.fatalln("resource source invalid:", cpy.SourceUrl)
		return
	}
	err := copyPath(cpy.DestPath, sourcePath)
	if err != nil {
		r.fatalln("resource copy failed:", cpy.SourceUrl, cpy.DestPath, err)
		return
	}
}

func (r runner) runActionTemplate(action string, envs *ExpandEnvs) {
	actions, ok := r.searchTemplate(action)
	if !ok {
		r.fatalln("template not found:", action)
		return
	}
	r.addIndent().runActions(envs, actions)
}

func (r runner) runActionCondition(action ActionCondition, envs *ExpandEnvs) {
	var validateCondition func(c *Condition) bool
	validateCondition = func(c *Condition) bool {
		n := 0
		if c.ConditionIf != nil {
			n++
		}
		if c.Not != nil {
			n++
		}
		if len(c.And) > 0 {
			n++
		}
		if len(c.Or) > 0 {
			n++
		}
		if n != 1 {
			return false
		}
		if c.ConditionIf != nil {
			return true
		}
		if c.Not != nil {
			return validateCondition(c.Not)
		}
		for i := range c.And {
			if !validateCondition(&c.And[i]) {
				return false
			}
		}
		for i := range c.Or {
			if !validateCondition(&c.Or[i]) {
				return false
			}
		}
		return true
	}
	var evalCondition func(c *Condition) bool
	evalCondition = func(c *Condition) bool {
		if c.ConditionIf != nil {
			value := c.ConditionIf.Value
			err := envs.expandStrings(&value, &c.ConditionIf.Compare)
			if err != nil {
				r.fatalln(err)
			}
			return checkCondition(&conditionContext{
				log:           r.log(),
				envs:          envs,
				valueOrigin:   c.ConditionIf.Value,
				compareOrigin: c.ConditionIf.Compare,
			}, value, c.ConditionIf.Operator, c.ConditionIf.Compare)
		}
		if c.Not != nil {
			return !evalCondition(c.Not)
		}
		if len(c.And) > 0 {
			for i := range c.And {
				if !evalCondition(&c.And[i]) {
					return false
				}
			}
			return true
		}
		if len(c.Or) > 0 {
			for i := range c.Or {
				if evalCondition(&c.Or[i]) {
					return true
				}
			}
			return false
		}
		panic(fmt.Errorf("unreachable"))
	}
	{
		var n int
		if action.ConditionCase != nil && action.ConditionCase.Default {
			n++
		}
		for i := range action.Cases {
			if action.Cases[i].Default {
				n++
			}
		}
		if n > 1 {
			r.fatalln("multiple default cases is not allowed.")
		}
	}
	var defaultCase *ConditionCase
	checkCase := func(i int, c *ConditionCase) bool {
		if c.Default {
			defaultCase = c
		}
		if c.Condition != nil {
			if !validateCondition(c.Condition) {
				r.fatalln("invalid condition case at seq:", i)
			}
			if evalCondition(c.Condition) {
				if i == 0 {
					r.debugln("action condition passed")
				} else {
					r.debugln("action condition case passed at seq:", i)
				}
				r.addIndentIfDebug().runActions(envs, c.Actions)
				return true
			}
		}
		return false
	}
	if action.ConditionCase != nil {
		if checkCase(0, action.ConditionCase) {
			return
		}
	}
	for i := range action.Cases {
		c := &action.Cases[i]
		if checkCase(i+1, c) {
			return
		}
	}
	if defaultCase != nil {
		r.debugln("action condition run default case")
		r.addIndentIfDebug().runActions(envs, defaultCase.Actions)
	} else {
		r.debugln("action condition doesn't passed")
	}
}
func (r runner) runActionSwitch(action ActionSwitch, envs *ExpandEnvs) {
	{
		var n int
		for i := range action.Cases {
			if action.Cases[i].Default {
				n++
			}
		}
		if n > 1 {
			r.fatalln("multiple default cases is not allowed")
		}
	}
	value := action.Value
	err := envs.expandStrings(&value)
	if err != nil {
		r.fatalln(err)
	}
	var defaultCase SwitchCase
	for _, c := range action.Cases {
		if c.Default {
			defaultCase = c
		}
		if c.Compare != nil {
			compare := *c.Compare
			err := envs.expandStrings(&compare)
			if err != nil {
				r.fatalln(err)
			}
			if checkCondition(&conditionContext{
				log:           r.log(),
				envs:          envs,
				valueOrigin:   action.Value,
				compareOrigin: *c.Compare,
			}, value, action.Operator, compare) {
				r.debugln("action switch case run:", *c.Compare)
				r.addIndentIfDebug().runActions(envs, c.Actions)
				return
			}
		}
	}
	if defaultCase.Default {
		r.debugln("action switch run default case")
		r.addIndent().runActions(envs, defaultCase.Actions)
	} else {
		r.debugln("action switch no case matched")
	}
}
func (r runner) runActionLoop(action ActionLoop, envs *ExpandEnvs) {
	var looper func(fn func(v string))
	switch {
	case action.Times > 0:
		looper = func(fn func(v string)) {
			for i := 0; i < action.Times; i++ {
				fn(strconv.Itoa(i))
			}
		}
	case action.Seq.From != action.Seq.To:
		step := action.Seq.Step
		if step == 0 {
			step = 1
		}
		delta := action.Seq.To - action.Seq.From
		if delta%step != 0 || delta/step < 0 {
			r.fatalln("invalid loop seq:", action.Seq.From, action.Seq.To, step)
		}
		looper = func(fn func(v string)) {
			for i := action.Seq.From; i != action.Seq.To; i += step {
				fn(strconv.Itoa(i))
			}
		}
	case len(action.Array) > 0:
		for i := range action.Array {
			err := envs.expandStrings(&action.Array[i])
			if err != nil {
				r.fatalln(err)
			}
		}
		looper = func(fn func(v string)) {
			for _, v := range action.Array {
				fn(v)
			}
		}
	case action.Split.Value != "":
		err := envs.expandStrings(&action.Split.Value)
		if err != nil {
			r.fatalln(err)
		}
		sep := action.Split.Separator
		if sep == "" {
			sep = " "
		}
		secs := stringSplitAndTrimFilterSpace(action.Split.Value, sep)
		looper = func(fn func(v string)) {
			for _, v := range secs {
				fn(v)
			}
		}
	default:
		r.fatalln("empty loop block")
		return
	}
	looper(func(v string) {
		envs := envs
		r := r.addIndentIfDebug()
		if action.Env != "" {
			envs.add(r.log(), action.Env, v, false)

			r.debugln("loop run with env:", action.Env+"="+v)
		}
		r.runActions(envs, action.Actions)
	})
}
func (r runner) runActionCmd(action ActionCmd, envs *ExpandEnvs) {
	var fds commandFds
	var err error
	if action.Stdin != "" {
		fds.Stdin, err = os.OpenFile(action.Stdin, os.O_RDONLY, 0)
		if err != nil {
			r.fatalln("open stdin failed:", err)
		}
	}
	openFile := func(name string, append bool) (*os.File, error) {
		flags := os.O_WRONLY | os.O_CREATE
		if append {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		err = os.MkdirAll(filepath.Dir(name), 0755)
		if err != nil {
			return nil, fmt.Errorf("create parent directories failed: %w", err)
		}
		return os.OpenFile(name, flags, 00644)
	}
	if action.Stdout != "" {
		out, err := openFile(action.Stdout, action.StdoutAppend)
		if err != nil {
			r.fatalln("open stdout file failed:", err)
		}
		defer out.Close()
		fds.Stdout = out
	}
	if action.Stderr != "" {
		if action.Stderr == action.Stdout {
			if action.StderrAppend != action.StdoutAppend {
				r.fatalln("couldn't open same stdout/stderr file in different append mode")
			} else {
				action.Stderr = action.Stdout
			}
		} else {
			out, err := openFile(action.Stderr, action.StderrAppend)
			if err != nil {
				r.fatalln("open stderr file failed:", err)
			}
			defer out.Close()
			fds.Stderr = out
		}
	}
	runCommand(r.log(), envs, action.Exec, false, fds)
}

func (r runner) runActions(envs *ExpandEnvs, a []Action) {
	for _, a := range a {
		if !reflect.DeepEqual(a.Env, Env{}) {
			r.debugln("Env")
			envs.parseEnvs(r.addIndentIfDebug().log(), []Env{a.Env})
		}
		if a.Cmd.Exec != "" {
			err := envs.expandStrings(&a.Cmd.Exec, &a.Cmd.Stdin, &a.Cmd.Stdout, &a.Cmd.Stderr)
			if err != nil {
				r.fatalln(err)
			}

			r.infoln("Cmd:", a.Cmd.Exec)
			r.runActionCmd(a.Cmd, envs)
		}
		if a.Copy.DestPath != "" {
			err := envs.expandStrings(&a.Copy.SourceUrl, &a.Copy.DestPath)
			if err != nil {
				r.fatalln(err)
			}
			r.infoln("Copy:", a.Copy.SourceUrl, a.Copy.DestPath)
			r.runActionCopy(a.Copy, envs)
		}
		if a.Del != "" {
			r.infoln("Del:", a.Del)
			err := os.RemoveAll(a.Del)
			if err != nil {
				r.fatalln("task action delete failed:", a.Del, err)
			}
		}
		if a.Replace.File != "" && len(a.Replace.Replaces) > 0 {
			r.infoln("Replace:", a.Replace.File)
			err := envs.expandStrings(&a.Replace.File)
			if err != nil {
				r.fatalln(err)
			}
			err = fileReplace(a.Replace.File, a.Replace.Replaces, a.Replace.Regexp)
			if err != nil {
				r.fatalln("task action replace failed:", a.Replace.File, err)
			}
		}
		if a.Chmod.Path != "" {
			r.infoln("Chmod:", a.Chmod.Path)
			err := envs.expandStrings(&a.Chmod.Path)
			if err != nil {
				r.fatalln(err)
			}
			err = os.Chmod(a.Chmod.Path, os.FileMode(a.Chmod.Mode))
			if err != nil {
				r.fatalln("chmod failed:", err)
			}
		}
		if len(a.Chdir.Actions) > 0 {
			r.infoln("Chdir:", a.Chdir.Dir)
			wd, err := os.Getwd()
			if err != nil {
				r.fatalln("get current directory failed:", err)
			}
			err = envs.expandStrings(&a.Chdir.Dir)
			if err != nil {
				r.fatalln(err)
			}
			err = os.Chdir(a.Chdir.Dir)
			if err != nil {
				r.fatalln("chdir failed:", err)
			}
			r.addIndent().runActions(envs, a.Chdir.Actions)
			err = os.Chdir(wd)
			if err != nil {
				r.fatalln("chdir back failed:", err)
			}
		}
		if a.Mkdir != "" {
			err := envs.expandStrings(&a.Mkdir)
			if err != nil {
				r.fatalln(err)
			}
			r.infoln("Mkdir:", a.Mkdir)
			err = os.MkdirAll(a.Mkdir, 0755)
			if err != nil {
				r.fatalln("mkdir failed:", err)
			}
		}
		if a.Template != "" {
			r.infoln("Template:", a.Template)
			r.runActionTemplate(a.Template, envs)
		}
		if len(a.Condition.Cases) > 0 || a.Condition.ConditionCase != nil {
			r.debugln("Condition")
			r.runActionCondition(a.Condition, envs)
		}
		if len(a.Switch.Cases) > 0 {
			r.debugln("Switch")
			r.runActionSwitch(a.Switch, envs)
		}
		if len(a.Loop.Actions) > 0 {
			r.debugln("Loop")
			r.runActionLoop(a.Loop, envs)
		}
		if len(a.Silent.Actions) > 0 {
			r.debugln("Silent")
			var (
				showLog    bool
				allowError bool
			)
			for _, flag := range a.Silent.Flags {
				switch flag {
				case SilentFlagShowLog:
					showLog = true
				case SilentFlagAllowError:
					allowError = true
				default:
					r.warnln("invalid silent flag:", flag)
				}
			}
			r.addIndentIfDebug().silent(!showLog, allowError).runActions(envs, a.Silent.Actions)
		}
	}
}
