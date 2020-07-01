package main

import (
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
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/mitchellh/go-ps"
	"github.com/zhuah/tash/syntax"
)

type indentLogger struct {
	indent     string
	debug      bool
	hideLog    bool
	allowError bool

	exit func()
}

func newLogger(debug bool) indentLogger {
	return indentLogger{
		debug: debug,
	}
}

func (w indentLogger) addIndent() indentLogger {
	nw := w
	nw.indent += "    "
	return nw
}

func (w indentLogger) addIndentIfDebug() indentLogger {
	if w.debug {
		return w.addIndent()
	}
	return w
}

func (w indentLogger) silent(hideLog, allowError bool) indentLogger {
	nw := w
	nw.hideLog = hideLog
	nw.allowError = allowError
	return nw
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
		if w.exit != nil {
			w.exit()
		} else {
			os.Exit(1)
		}
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

func listTasks(configs *Configuration, log indentLogger, taskNames []string, showArgs bool) {
	if len(configs.Tasks) == 0 {
		log.infoln("no tasks defined.")
	} else {
		if len(taskNames) == 0 {
			log.infoln("available tasks:")
			var names []string
			for name := range configs.Tasks {
				names = append(names, name)
			}
			sort.Strings(names)
			taskNames = names
		}
		llog := log.addIndent()
		for _, name := range taskNames {
			task, has := configs.Tasks[name]
			if !has {
				log.fatalln("task not found:", name)
				return
			}

			llog.infoln(fmt.Sprintf("- %s: %s", name, task.Description))
			if !showArgs {
				continue
			}

			alog := llog.addIndent()
			if len(task.Args) > 0 {
				alog.infoln("args:")
				for _, arg := range task.Args {
					alog.infoln(fmt.Sprintf("- %s: %s", arg.Env, arg.Description))
					if arg.Default != "" {
						alog.infoln(fmt.Sprintf("  default: '%s'", arg.Default))
					}
				}
			}
		}
	}
}

func runTasks(configs *Configuration, log indentLogger, names []string, args []string) {
	if len(names) == 0 {
		log.fatalln("no tasks to run")
		return
	}
	currDir, err := os.Getwd()
	if err != nil {
		log.fatalln("get current directory failed:", err)
		return
	}
	for _, name := range names {
		_, has := configs.Tasks[name]
		if !has {
			log.fatalln("task not found:", name)
			return
		}
	}

	r := newRunner(nil, log, configs)
	r.globalArgs = args
	for i, name := range names {
		if i > 0 {
			r.infoln() // create new line
		}
		r.runTaskByName(name, currDir)
	}
}

type runner struct {
	globalArgs []string
	parent     *runner

	indentLogger
	configs      *Configuration
	noExitOnFail bool

	failed bool
}

func newRunner(parent *runner, log indentLogger, configs *Configuration) *runner {
	r := runner{
		parent:       parent,
		indentLogger: log,
		configs:      configs,
	}
	r.indentLogger.exit = r.doExit
	return &r
}

func (r *runner) root() *runner {
	rt := r
	for rt.parent != nil {
		rt = rt.parent
	}
	return rt
}

func (r *runner) next(fn func()) {
	if !r.root().failed {
		fn()
	}
}

func (r *runner) doExit() {
	rt := r.root()
	rt.failed = true
	if !rt.noExitOnFail {
		os.Exit(1)
	}
}

func (r *runner) log() indentLogger {
	return r.indentLogger
}

func (r *runner) addIndent() *runner {
	return newRunner(r, r.log().addIndent(), r.configs)
}

func (r *runner) addIndentIfDebug() *runner {
	return newRunner(r, r.log().addIndentIfDebug(), r.configs)
}

func (r *runner) silent(hideLog, allowError bool) *runner {
	return newRunner(r, r.log().silent(hideLog, allowError), r.configs)
}

func (r *runner) searchTask(name string) (syntax.Task, bool) {
	task, ok := r.configs.Tasks[name]
	return task, ok
}

func (r *runner) searchTemplate(name string) ([]syntax.Action, bool) {
	tmpl, ok := r.configs.Templates[name]
	return tmpl, ok
}

func (r *runner) createTaskEnvs(name string, task syntax.Task, workDir string) *ExpandEnvs {
	envs := newExpandEnvs()
	r.debugln(">>>>> adds system environments")
	envs.parsePairs(r.log(), os.Environ(), false)
	if len(r.root().globalArgs) > 0 {
		r.debugln(">>>>> adds user provided arguments")
		for _, a := range r.root().globalArgs {
			blocks := splitBlocks(a)
			envs.parsePairs(r.log(), blocks, false)
		}
	}
	r.debugln(">>>>> adds builtin environments")
	envs.add(r.log(), syntax.BUILTIN_ENV_WORKDIR, workDir, false)
	envs.add(r.log(), syntax.BUILTIN_ENV_HOST_OS, runtime.GOOS, false)
	envs.add(r.log(), syntax.BUILTIN_ENV_HOST_ARCH, runtime.GOARCH, false)
	envs.add(r.log(), syntax.BUILTIN_ENV_TASK_NAME, name, false)
	if len(r.configs.Envs) > 0 {
		r.debugln(">>>>> add configuration environments")
		envs.parseEnvs(r.log(), r.configs.Envs)
	}
	if len(task.Args) > 0 {
		r.debugln(">>>>> checking task default arguments")
		for _, arg := range task.Args {
			if arg.Env == "" {
				r.fatalln("empty task argument name")
				return envs
			}

			val, err := envs.lookupAndFilter(arg.Env, nil)
			if err != nil {
				r.fatalln("lookup task argument value failed:", arg.Env, err)
				return envs
			}
			if val != "" {
				continue
			}

			err = envs.expandStringPtrs(&arg.Default)
			if err != nil {
				r.fatalln("expand task args failed:", arg.Env, err)
				return envs
			}

			r.debugln("uses task argument default value:", arg.Env)
			envs.add(r.log(), arg.Env, arg.Default, false)
		}
	}

	return envs
}

func (r *runner) runTask(name string, task syntax.Task, baseDir string) {
	workDir := stringToSlash(filepath.Join(baseDir, task.WorkDir))
	err := os.Chdir(workDir)
	if err != nil {
		r.fatalln("change working directory failed:", err)
		return
	}

	r.infoln("workdir:", workDir)
	envs := r.createTaskEnvs(name, task, workDir)
	r.runActions(envs, task.Actions)
}

func (r *runner) runTaskByName(name, baseDir string) {
	r.infoln("Task:", name)
	task, ok := r.searchTask(name)
	if !ok {
		r.fatalln("task not found:", name)
		return
	}

	r.addIndent().runTask(name, task, baseDir)
}

func (r *runner) resourceNeedsSync(cpy syntax.ActionCopy) bool {
	info, err := os.Stat(cpy.DestPath)
	if err != nil {
		return true
	}
	if cpy.Hash.Sig == "" || info.IsDir() { // always sync
		return true
	}

	fd, err := os.OpenFile(cpy.DestPath, os.O_RDONLY, 0)
	if err != nil {
		return true
	}
	defer fd.Close()

	return checkHash(r.log(), cpy.DestPath, cpy.Hash.Alg, cpy.Hash.Sig, fd)
}

func (r *runner) resourceIsValid(res syntax.ActionCopy, path string) bool {
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

func (r *runner) runActionCopy(cpy syntax.ActionCopy, envs *ExpandEnvs) {
	if !r.resourceNeedsSync(cpy) {
		r.debugln("resource reuse.")
		return
	}
	var sourcePath string
	if strings.Contains(cpy.SourceUrl, "://") {
		sourceUrl := cpy.SourceUrl
		ul, err := url.Parse(sourceUrl)
		if err != nil {
			r.fatalln("couldn't parse source url:", sourceUrl, err)
			return
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
			return
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

func (r *runner) runActionTemplate(action string, envs *ExpandEnvs) {
	actions, ok := r.searchTemplate(action)
	if !ok {
		r.fatalln("template not found:", action)
		return
	}
	r.addIndent().runActions(envs, actions)
}

func (r *runner) validateCondition(c *syntax.Condition) bool {
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
		return r.validateCondition(c.Not)
	}
	for i := range c.And {
		if !r.validateCondition(&c.And[i]) {
			return false
		}
	}
	for i := range c.Or {
		if !r.validateCondition(&c.Or[i]) {
			return false
		}
	}
	return true
}

func (r *runner) evalCondition(c *syntax.Condition, envs *ExpandEnvs) bool {
	if c.ConditionIf != nil {
		value := c.ConditionIf.Value
		err := envs.expandStringPtrs(&value, c.ConditionIf.Compare)
		if err != nil {
			r.fatalln(err)
			return false
		}
		ok, err := checkCondition(envs, value, c.ConditionIf.Operator, c.ConditionIf.Compare)
		if err != nil {
			r.fatalln("check condition failed:", err)
			return false
		}
		return ok
	}
	if c.Not != nil {
		return !r.evalCondition(c.Not, envs)
	}
	if len(c.And) > 0 {
		for i := range c.And {
			if !r.evalCondition(&c.And[i], envs) {
				return false
			}
		}
		return true
	}
	if len(c.Or) > 0 {
		for i := range c.Or {
			if r.evalCondition(&c.Or[i], envs) {
				return true
			}
		}
		return false
	}
	panic(fmt.Errorf("unreachable"))
}

func (r *runner) runActionCondition(action syntax.ActionCondition, envs *ExpandEnvs) {
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
			return
		}
	}
	var defaultCase *syntax.ConditionCase
	checkCase := func(i int, c *syntax.ConditionCase) bool {
		if c.Default {
			defaultCase = c
		}
		if c.Condition != nil {
			if !r.validateCondition(c.Condition) {
				r.fatalln("invalid condition case at seq:", i)
				return false
			}
			if r.evalCondition(c.Condition, envs) {
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

func (r *runner) runActionSwitch(action syntax.ActionSwitch, envs *ExpandEnvs) {
	{
		var n int
		for i := range action.Cases {
			if action.Cases[i].Default {
				n++
			}
		}
		if n > 1 {
			r.fatalln("multiple default cases is not allowed")
			return
		}
	}
	value := action.Value
	err := envs.expandStringPtrs(&value)
	if err != nil {
		r.fatalln(err)
		return
	}
	var defaultCase syntax.SwitchCase
	for _, c := range action.Cases {
		if c.Default {
			defaultCase = c
		}
		if c.Compare != nil {
			compare := *c.Compare
			err := envs.expandStringPtrs(&compare)
			if err != nil {
				r.fatalln(err)
				return
			}
			ok, err := checkCondition(envs, value, action.Operator, c.Compare)
			if err != nil {
				r.fatalln("check condition failed:", err)
				return
			}
			if ok {
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

func (r *runner) runActionIf(action syntax.ActionIf, envs *ExpandEnvs) {
	if !r.validateCondition(action.Condition) {
		r.fatalln("invalid condition")
		return
	}

	if r.evalCondition(action.Condition, envs) {
		r.debugln("action if passed")
		r.addIndentIfDebug().runActions(envs, action.OK)
	} else {
		r.debugln("action if failed")
		r.addIndentIfDebug().runActions(envs, action.Fail)
	}
}

func (r *runner) runActionLoop(action syntax.ActionLoop, envs *ExpandEnvs) {
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
			return
		}
		looper = func(fn func(v string)) {
			for i := action.Seq.From; i != action.Seq.To; i += step {
				fn(strconv.Itoa(i))
			}
		}
	case len(action.Array) > 0:
		for i := range action.Array {
			err := envs.expandStringPtrs(&action.Array[i])
			if err != nil {
				r.fatalln(err)
				return
			}
		}
		looper = func(fn func(v string)) {
			for _, v := range action.Array {
				fn(v)
			}
		}
	case action.Split.Value != "":
		err := envs.expandStringPtrs(&action.Split.Value)
		if err != nil {
			r.fatalln(err)
			return
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

func (r *runner) runActionCmd(action syntax.ActionCmd, envs *ExpandEnvs) {
	envs.add(r.log(), syntax.BUILTIN_ENV_LAST_COMMAND_PID, "", false)

	var fds commandFds
	var err error
	if action.Stdin != "" {
		fds.Stdin, err = os.OpenFile(action.Stdin, os.O_RDONLY, 0)
		if err != nil {
			r.fatalln("open stdin failed:", err)
			return
		}
	}

	if action.Stdout != "" {
		out, err := openFile(action.Stdout, action.StdoutAppend)
		if err != nil {
			r.fatalln("open stdout file failed:", err)
			return
		}
		defer out.Close()
		fds.Stdout = out
	}
	if action.Stderr != "" {
		if action.Stderr == action.Stdout {
			if action.StderrAppend != action.StdoutAppend {
				r.fatalln("couldn't open same stdout/stderr file in different append mode")
				return
			} else {
				action.Stderr = action.Stdout
			}
		} else {
			out, err := openFile(action.Stderr, action.StderrAppend)
			if err != nil {
				r.fatalln("open stderr file failed:", err)
				return
			}
			defer out.Close()
			fds.Stderr = out
		}
	}
	pid, _, err := runCommand(envs, action.Exec, action.WorkDir, false, fds, action.Background)
	if err != nil {
		r.fatalln("run command failed:", err)
		return
	}
	envs.add(r.log(), syntax.BUILTIN_ENV_LAST_COMMAND_PID, strconv.Itoa(pid), false)
}

func (r *runner) runActionWatch(action syntax.ActionWatch, envs *ExpandEnvs) {
	err := envs.expandStringPtrs(&action.Dirs, &action.Files)
	if err != nil {
		r.fatalln(err)
		return
	}

	dirs := splitBlocks(action.Dirs)
	files := splitBlocks(action.Files)
	w, err := newWatcher(r.log(), dirs, files)
	if err != nil {
		r.fatalln("create watcher failed:", err)
		return
	}
	defer w.close()

	r.infoln("start watching.")
	w.run(func() {
		nr := newRunner(nil, r.log().addIndent(), r.configs)
		nr.noExitOnFail = true
		nr.infoln("received fs changes, run watcher actions >>>>>>")
		nr.runActions(envs, action.Actions)
		nr.infoln()
	})
}

func (r *runner) findProcess(pidstr, name string, envs *ExpandEnvs) (*os.Process, bool) {
	err := envs.expandStringPtrs(&name, &pidstr)
	if err != nil {
		r.fatalln(err)
		return nil, false
	}

	var pid int
	if pidstr != "" {
		pid, err = strconv.Atoi(pidstr)
		if err != nil {
			r.warnln("convert pid to number failed:", pidstr, err)
			return nil, false
		}
	}
	if name != "" {
		processes, err := ps.Processes()
		if err != nil {
			r.warnln("list processes failed:", err)
			return nil, false
		}
		processName := strings.TrimSuffix(name, ".exe")
		for _, p := range processes {
			if pid > 0 && p.Pid() != pid {
				continue
			}
			name := strings.TrimSuffix(filepath.ToSlash(p.Executable()), ".exe")
			if name == processName || strings.HasSuffix(name, "/"+processName) {
				pid = p.Pid()
				break
			}
		}
	}
	if pid <= 0 {
		r.warnln("couldn't find process")
		return nil, false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		r.warnln("couldn't find process:", err)
		return nil, false
	}
	return process, true
}

func (r *runner) runActionPkill(action syntax.ActionPkill, envs *ExpandEnvs) {
	err := envs.expandStringPtrs(&action.Signal)
	if err != nil {
		r.fatalln(err)
		return
	}

	if action.Signal == "" {
		action.Signal = "TERM"
	}
	sig, has := signals[action.Signal]
	if !has {
		n, err := strconv.ParseUint(action.Signal, 10, 32)
		if err != nil || n <= 0 {
			r.fatalln("invalid signal number:", action.Signal, err)
			return
		}
		sig = syscall.Signal(n)
	}
	process, ok := r.findProcess(action.Pid, action.Process, envs)
	if !ok {
		return
	}
	err = process.Signal(sig)
	if err != nil {
		r.warnln("signal process failed:", err)
	}
}

func (r *runner) runActionWait(action syntax.ActionWait, envs *ExpandEnvs) {
	process, ok := r.findProcess(action.Pid, action.Process, envs)
	if !ok {
		return
	}
	_, err := process.Wait()
	if err != nil {
		r.warnln("couldn't wait on process:", process.Pid, err)
		return
	}
}

func (r *runner) runActionTask(name string, passEnvs, returnEnvs []string, envs *ExpandEnvs) {
	wd, err := os.Getwd()
	if err != nil {
		r.fatalln("get current directory failed:", err)
		return
	}

	r.infoln("workdir:", wd)
	task, ok := r.searchTask(name)
	if !ok {
		r.fatalln("task not found:", name)
		return
	}
	nr := newRunner(nil, r.log().addIndent(), r.configs)
	nr.noExitOnFail = true

	taskEnvs := r.createTaskEnvs(name, task, wd)
	transferEnvs := func(from, to *ExpandEnvs, envs []string) {
		for _, env := range envs {
			v, _ := from.lookupAndFilter(env, nil)
			to.add(nr.log(), env, v, false)
		}
	}
	transferEnvs(envs, taskEnvs, passEnvs)
	nr.runActions(taskEnvs, task.Actions)
	if !nr.failed {
		transferEnvs(taskEnvs, envs, returnEnvs)
	}
	err = os.Chdir(wd)
	if err != nil {
		r.fatalln("chdir back failed:", err)
		return
	}
	if nr.failed {
		r.fatalln("child task failed")
	}
}

func (r *runner) expandPathBlockAndGlob(path string, envs *ExpandEnvs, mustBeFile bool) ([]string, bool) {
	err := envs.expandStringPtrs(&path)
	if err != nil {
		r.fatalln(err)
		return nil, false
	}
	matched, err := splitBlocksAndGlobPath(path, mustBeFile)
	if err != nil {
		r.fatalln("glob path failed:", err)
		return nil, false
	}
	return matched, true
}

func (r *runner) runActions(envs *ExpandEnvs, a []syntax.Action) {
	for _, a := range a {
		r.next(func() {
			if !reflect.DeepEqual(a.Env, syntax.Env{}) {
				r.debugln("Env")
				envs.parseEnvs(r.addIndentIfDebug().log(), []syntax.Env{a.Env})
			}
		})
		r.next(func() {
			if a.Cmd.Exec != "" {
				err := envs.expandStringPtrs(&a.Cmd.Exec, &a.Cmd.WorkDir, &a.Cmd.Stdin, &a.Cmd.Stdout, &a.Cmd.Stderr)
				if err != nil {
					r.fatalln(err)
					return
				}

				r.infoln("Cmd:", a.Cmd.Exec)
				r.runActionCmd(a.Cmd, envs)
			}
		})
		r.next(func() {
			if a.Copy.DestPath != "" {
				err := envs.expandStringPtrs(&a.Copy.SourceUrl, &a.Copy.DestPath)
				if err != nil {
					r.fatalln(err)
				}
				ptrsToSlash(&a.Copy.SourceUrl, &a.Copy.DestPath)
				r.infoln("Copy:", a.Copy.SourceUrl, a.Copy.DestPath)
				r.runActionCopy(a.Copy, envs)
			}
			return
		})
		r.next(func() {
			if a.Del != "" {
				matched, ok := r.expandPathBlockAndGlob(a.Del, envs, false)
				if !ok {
					return
				}
				r.infoln("Del:", matched)
				for _, m := range matched {
					err := os.RemoveAll(m)
					if err != nil {
						r.fatalln("task action delete failed:", m, err)
					}
				}
			}
		})
		r.next(func() {
			if a.Replace.File != "" {
				if len(a.Replace.Replaces) <= 0 || len(a.Replace.Replaces)%2 != 0 {
					r.fatalln("invalid replaces pairs")
				}
				matched, ok := r.expandPathBlockAndGlob(a.Replace.File, envs, true)
				if !ok {
					return
				}
				r.infoln("Replace:", matched)
				replacer, err := fileReplacer(a.Replace.Replaces, a.Replace.Regexp)
				if err != nil {
					r.fatalln("build replacer failed:", err)
					return
				}
				for _, m := range matched {
					err = replacer(m)
					if err != nil {
						r.fatalln("replace file failed:", a.Replace.File, err)
					}
				}
			}
		})
		r.next(func() {
			if a.Chmod.Path != "" {
				matched, ok := r.expandPathBlockAndGlob(a.Chmod.Path, envs, false)
				if !ok {
					return
				}
				r.infoln("Chmod:", matched)
				for _, m := range matched {
					err := os.Chmod(m, os.FileMode(a.Chmod.Mode))
					if err != nil {
						r.fatalln("chmod failed:", m, err)
					}
				}
			}
		})
		r.next(func() {
			if len(a.Chdir.Actions) > 0 {
				err := envs.expandStringPtrs(&a.Chdir.Dir)
				if err != nil {
					r.fatalln(err)
					return
				}
				r.infoln("Chdir:", a.Chdir.Dir)
				err = runInDir(a.Chdir.Dir, func() error {
					r.addIndent().runActions(envs, a.Chdir.Actions)
					return nil
				})
				if err != nil {
					r.fatalln("chdir failed:", err)
					return
				}
			}
		})
		r.next(func() {
			if a.Mkdir != "" {
				err := envs.expandStringPtrs(&a.Mkdir)
				if err != nil {
					r.fatalln(err)
					return
				}
				blocks := splitBlocks(a.Mkdir)

				r.infoln("Mkdir:", blocks)
				for _, dir := range blocks {
					err = os.MkdirAll(dir, 0755)
					if err != nil {
						r.fatalln("mkdir failed:", err)
						return
					}
				}
			}
		})
		r.next(func() {
			if a.Template != "" {
				templates := splitBlocks(a.Template)
				r.infoln("Template:", templates)
				for _, template := range templates {
					if len(templates) > 1 {
						r.infoln(">>>>> template:", template)
					}
					r.runActionTemplate(template, envs)
				}
			}
		})
		r.next(func() {
			if len(a.Condition.Cases) > 0 || a.Condition.ConditionCase != nil {
				r.debugln("Condition")
				r.runActionCondition(a.Condition, envs)
			}
		})
		r.next(func() {
			if len(a.Switch.Cases) > 0 {
				r.debugln("Switch")
				r.runActionSwitch(a.Switch, envs)
			}
		})
		r.next(func() {
			if len(a.If.OK) > 0 || len(a.If.Fail) > 0 {
				r.debugln("If")
				r.runActionIf(a.If, envs)
			}
		})
		r.next(func() {
			if len(a.Loop.Actions) > 0 {
				r.debugln("Loop")
				r.runActionLoop(a.Loop, envs)
			}
		})
		r.next(func() {
			if len(a.Silent.Actions) > 0 {
				r.debugln("Silent")
				var (
					showLog    bool
					allowError bool
				)
				for _, flag := range a.Silent.Flags {
					switch flag {
					case syntax.SilentFlagShowLog:
						showLog = true
					case syntax.SilentFlagAllowError:
						allowError = true
					default:
						r.warnln("invalid silent flag:", flag)
					}
				}
				r.addIndentIfDebug().silent(!showLog, allowError).runActions(envs, a.Silent.Actions)
			}
		})
		r.next(func() {
			if a.Echo != (syntax.ActionEcho{}) {
				err := envs.expandStringPtrs(&a.Echo.File, &a.Echo.Content)
				if err != nil {
					r.fatalln(err)
					return
				}
				r.infoln("Echo:", a.Echo.File)
				func() {
					fd, err := openFile(a.Echo.File, a.Echo.Append)
					if err != nil {
						r.fatalln("open file failed:", err)
						return
					}
					defer fd.Close()
					_, err = fd.WriteString(a.Echo.Content)
					if err != nil {
						r.warnln("write file failed:", err)
					}
				}()
			}
		})
		r.next(func() {
			if a.Task.Name != "" {
				err := envs.expandStringPtrs(&a.Task.Name)
				if err != nil {
					r.fatalln(err)
					return
				}
				tasks := splitBlocks(a.Task.Name)
				r.infoln("Task:", tasks)
				for _, name := range tasks {
					if len(tasks) > 1 {
						r.infoln(">>>>>task:", name)
					}
					r.runActionTask(name, a.Task.PassEnvs, a.Task.ReturnEnvs, envs)
				}
			}
		})
		r.next(func() {
			if len(a.Watch.Actions) > 0 {
				r.infoln("Watch.")
				r.runActionWatch(a.Watch, envs)
			}
		})
		r.next(func() {
			if a.Pkill != (syntax.ActionPkill{}) {
				r.infoln("Pkill.")

				r.runActionPkill(a.Pkill, envs)
			}
		})
		r.next(func() {
			if a.Sleep > 0 {
				dur := time.Duration(a.Sleep) * time.Millisecond
				r.infoln("Sleep:", dur.String())

				time.Sleep(dur)
			}
		})
		r.next(func() {
			if a.Wait != (syntax.ActionWait{}) {
				r.infoln("Wait.")

				r.runActionWait(a.Wait, envs)
			}
		})
		r.next(func() {
			if a.Warn != "" {
				r.debugln("Warn.")
				err := envs.expandStringPtrs(&a.Warn)
				if err != nil {
					r.fatalln(err)
					return
				}
				r.warnln(a.Warn)
			}
		})
		r.next(func() {
			if a.Fatal != "" {
				r.debugln("Fatal.")
				err := envs.expandStringPtrs(&a.Fatal)
				if err != nil {
					r.fatalln(err)
					return
				}
				r.fatalln(a.Fatal)
			}
		})
	}
}
