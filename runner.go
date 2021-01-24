package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/mitchellh/go-ps"
	"github.com/uiez/tash/syntax"
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

func (r *runner) searchTemplate(name string) (syntax.ActionList, bool) {
	tmpl, ok := r.configs.Templates[name]
	return tmpl, ok
}

func (r *runner) createTaskEnvs(name string, task syntax.Task, workDir string) *ExpandEnvs {
	envs := newExpandEnvs()
	r.debugln(">>>>> adds system environments")
	envs.parsePairs(r.log(), os.Environ(), false)
	r.debugln(">>>>> adds builtin environments")
	envs.addAndExpand(r.log(), syntax.BUILTIN_ENV_WORKDIR, workDir, false)
	envs.addAndExpand(r.log(), syntax.BUILTIN_ENV_HOST_OS, runtime.GOOS, false)
	envs.addAndExpand(r.log(), syntax.BUILTIN_ENV_HOST_ARCH, runtime.GOARCH, false)
	envs.addAndExpand(r.log(), syntax.BUILTIN_ENV_TASK_NAME, name, false)
	envs.addAndExpand(r.log(), syntax.BUILTIN_ENV_PATHLISTSEP, string(os.PathListSeparator), false)

	userArgsEnv := envs.copy()
	if len(r.root().globalArgs) > 0 {
		r.debugln(">>>>> adds user provided arguments")
		for _, a := range r.root().globalArgs {
			blocks := splitBlocks(a)
			userArgsEnv.parsePairs(r.log(), blocks, false)
		}
	}
	if len(task.Args) > 0 {
		r.debugln(">>>>> checking task default arguments")
		for _, arg := range task.Args {
			if arg.Env == "" {
				r.fatalln("empty task argument name")
				return envs
			}

			val, err := userArgsEnv.lookupAndFilter(arg.Env, nil)
			if err != nil {
				r.fatalln("lookup task argument value failed:", arg.Env, err)
				return envs
			}
			if val == "" {
				val = arg.Default
				err = envs.expandStringPtrs(&val)
				if err != nil {
					r.fatalln("expand task args failed:", arg.Env, err)
					return envs
				}
				r.debugln("uses task argument default value:", arg.Env)
			}
			envs.addAndExpand(r.log(), arg.Env, val, false)
		}
	}

	if r.configs.Env.Length() > 0 {
		r.debugln(">>>>> add configuration environments")
		envs.parseEnv(r.log(), r.configs.Env)
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

func (r *runner) resourceNeedsSync(cpy syntax.ActionCopy, isLocalFile bool) bool {
	info, err := os.Stat(cpy.DestPath)
	if err != nil {
		return true
	}
	if info.IsDir() {
		return true
	}
	if cpy.Hash.Sig == "" {
		return isLocalFile
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
	var (
		sourcePath  string
		needsRemove bool
		force       bool
	)
	if cpy.Force != "" {
		val, err := envs.expandString(cpy.Force)
		if err != nil {
			r.fatalln(err)
			return
		}
		ok, err := checkCondition(envs, val, "", nil)
		if err != nil {
			r.fatalln("couldn't eval value of 'force' field:", cpy.Force, err)
			return
		}
		force = ok

		if force {
			r.debugln("force sync resource.")
		}
	}
	if strings.Contains(cpy.SourceUrl, "://") {
		sourceUrl := cpy.SourceUrl
		ul, err := url.Parse(sourceUrl)
		if err != nil {
			r.fatalln("couldn't parse source url:", sourceUrl, err)
			return
		}
		switch ul.Scheme {
		case "file":
			if !force && !r.resourceNeedsSync(cpy, true) {
				r.debugln("resource reuse.")
				return
			}
			sourcePath = ul.Path
			if runtime.GOOS == "windows" {
				sourcePath = strings.TrimPrefix(sourcePath, "/")
			}
		case "http", "https":
			if !force && !r.resourceNeedsSync(cpy, false) {
				r.debugln("resource reuse.")
				return
			}
			path, err := downloadFile(cpy.SourceUrl)
			if err != nil {
				r.fatalln("download file failed:", cpy.SourceUrl, err)
				return
			}
			sourcePath = path
			needsRemove = true
		default:
			r.fatalln("unsupported source url schema:", ul.Scheme)
			return
		}
	} else {
		if !force && !r.resourceNeedsSync(cpy, true) {
			r.debugln("resource reuse.")
			return
		}
		sourcePath = cpy.SourceUrl
	}
	defer func() {
		if needsRemove {
			os.Remove(sourcePath)
		}
	}()
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

func (r *runner) runActionSwitch(action syntax.ActionSwitch, envs *ExpandEnvs) {
	{
		var n int
		for compare := range action.Cases {
			if compare == action.Default {
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
	var defaultActions syntax.ActionList
	for compare, actions := range action.Cases {
		if compare == action.Default {
			defaultActions = actions
		} else {
			err := envs.expandStringPtrs(&compare)
			if err != nil {
				r.fatalln(err)
				return
			}
			ok, err := checkCondition(envs, value, action.Operator, &compare)
			if err != nil {
				r.fatalln("check condition failed:", err)
				return
			}
			if ok {
				r.debugln("action switch case run:", compare)
				r.addIndentIfDebug().runActions(envs, actions)
				return
			}
		}
	}
	if defaultActions.Length() > 0 {
		r.debugln("action switch run default case")
		r.addIndent().runActions(envs, defaultActions)
	} else {
		r.debugln("action switch no case matched")
	}
}

func (r *runner) runActionIf(action syntax.ActionIf, envs *ExpandEnvs) {
	val, err := envs.expandString(action.Check)
	if err != nil {
		r.fatalln(err)
	}
	ok, err := checkCondition(envs, val, "", nil)
	if err != nil {
		r.fatalln("check condition failed:", err)
	}
	if ok {
		r.debugln("action if passed")
		r.addIndentIfDebug().runActions(envs, action.Actions)
	} else {
		r.debugln("action if failed")
		r.addIndentIfDebug().runActions(envs, action.Else)
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

		var (
			varEnvVal   string
			varEnvExist bool
		)
		if action.Var != "" {
			varEnvVal, varEnvExist = envs.get(action.Var)
			envs.set(action.Var, v)

			r.debugln("loop run with var:", action.Var+"="+v)
		}
		r.runActions(envs, action.Actions)
		if varEnvExist { // restore
			envs.set(action.Var, varEnvVal)
		}
	})
}

func (r *runner) runActionCmd(action syntax.ActionCmd, envs *ExpandEnvs, execs []string) {
	envs.addAndExpand(r.log(), syntax.BUILTIN_ENV_LAST_COMMAND_PID, "", false)

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

	cmdEnvs := envs
	if action.Env.Length() > 0 {
		cmdEnvs = envs.copy()
		r.debugln(">>>>> add command local environments")
		cmdEnvs.parseEnv(r.log(), action.Env)
	}
	for _, exec := range execs {
		if exec != "" {
			r.infoln("exec:", exec)
			pid, _, err := runCommand(cmdEnvs, exec, action.WorkDir, false, fds, action.Background)
			if err != nil {
				r.fatalln("run command failed:", err)
				return
			}
			envs.addAndExpand(r.log(), syntax.BUILTIN_ENV_LAST_COMMAND_PID, strconv.Itoa(pid), false)
		}
	}
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
			to.addAndExpand(nr.log(), env, v, false)
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

func (r *runner) runActions(envs *ExpandEnvs, a syntax.ActionList) {
	for _, a := range a.Actions() {
		if a.On != "" {
			val, err := envs.expandString(a.On)
			if err != nil {
				r.fatalln(err)
			}
			ok, err := checkCondition(envs, val, "", nil)
			if err != nil {
				r.fatalln("check condition failed:", err)
			}
			if !ok {
				r.debugln("action condition failed")
				continue
			}

			r.debugln("action condition passed")
		}
		var done bool
		next := func(cond bool, fn func()) {
			if cond && !done && !r.root().failed {
				fn()
				done = true
			}
		}
		next(a.Env.Length() > 0, func() {
			r.debugln("Env")
			envs.parseEnv(r.addIndentIfDebug().log(), a.Env)
		})
		next(a.Cmd.Exec != "", func() {
			err := envs.expandStringPtrs(&a.Cmd.Exec, &a.Cmd.WorkDir, &a.Cmd.Stdin, &a.Cmd.Stdout, &a.Cmd.Stderr)
			if err != nil {
				r.fatalln(err)
				return
			}

			execs := stringSplitAndTrim(a.Cmd.Exec, "\n")
			r.infoln("Cmd")
			r.addIndent().runActionCmd(a.Cmd, envs, execs)
		})
		next(a.Copy.DestPath != "", func() {
			err := envs.expandStringPtrs(&a.Copy.SourceUrl, &a.Copy.DestPath)
			if err != nil {
				r.fatalln(err)
			}
			ptrsToSlash(&a.Copy.SourceUrl, &a.Copy.DestPath)
			r.infoln("Copy:", a.Copy.SourceUrl, a.Copy.DestPath)
			r.addIndentIfDebug().runActionCopy(a.Copy, envs)
		})
		next(a.Del != "", func() {
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
		})
		next(a.Replace.File != "", func() {
			if len(a.Replace.Replaces) <= 0 || len(a.Replace.Replaces)%2 != 0 {
				r.fatalln("invalid replaces pairs")
			}
			matched, ok := r.expandPathBlockAndGlob(a.Replace.File, envs, true)
			if !ok {
				return
			}
			r.infoln("Replace:", matched)
			r.debugln("Replacements:", a.Replace.Replaces)
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
		})
		next(a.Chmod.Path != "", func() {
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
		})
		next(a.Chdir.Actions.Length() > 0, func() {
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
		})
		next(a.Mkdir != "", func() {
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
		})
		next(a.Template != "", func() {
			templates := splitBlocks(a.Template)
			r.infoln("Template:", templates)
			for _, template := range templates {
				if len(templates) > 1 {
					r.infoln(">>>>> template:", template)
				}
				r.runActionTemplate(template, envs)
			}
		})
		next(len(a.Switch.Cases) > 0, func() {
			r.debugln("Switch")
			r.runActionSwitch(a.Switch, envs)
		})
		next(a.If.Actions.Length() > 0 || a.If.Else.Length() > 0, func() {
			r.debugln("If")
			r.addIndentIfDebug().runActionIf(a.If, envs)
		})
		next(a.Loop.Actions.Length() > 0, func() {
			r.debugln("Loop")
			r.runActionLoop(a.Loop, envs)
		})
		next(a.Silent.Actions.Length() > 0, func() {
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
		})
		next(a.Echo != (syntax.ActionEcho{}), func() {
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
		})
		next(a.Task.Name != "", func() {
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
		})
		next(a.Watch.Actions.Length() > 0, func() {
			r.infoln("Watch.")
			r.runActionWatch(a.Watch, envs)
		})
		next(a.Pkill != (syntax.ActionPkill{}), func() {
			r.infoln("Pkill.")

			r.runActionPkill(a.Pkill, envs)
		})
		next(a.Sleep > 0, func() {
			dur := time.Duration(a.Sleep) * time.Millisecond
			r.infoln("Sleep:", dur.String())

			time.Sleep(dur)
		})
		next(a.Wait != (syntax.ActionWait{}), func() {
			r.infoln("Wait.")

			r.runActionWait(a.Wait, envs)
		})
		next(a.Warn != "", func() {
			r.debugln("Warn.")
			err := envs.expandStringPtrs(&a.Warn)
			if err != nil {
				r.fatalln(err)
				return
			}
			r.warnln(a.Warn)
		})
		next(a.Fatal != "", func() {
			r.debugln("Fatal.")
			err := envs.expandStringPtrs(&a.Fatal)
			if err != nil {
				r.fatalln(err)
				return
			}
			r.fatalln(a.Fatal)
		})
	}
}
