package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/cosiner/flag"
	"github.com/fatih/color"
	"github.com/ghodss/yaml"
)

type Flags struct {
	Conf string `usage:"config file, default tash.yaml in current/ancestor directory"`
	List struct {
		Enable bool
		Detail bool `names:"-d,--detail" default:"false" usage:"show task descriptions"`
	} `usage:"list available tasks"`

	Debug bool     `names:"--debug" usage:"show debug messages"`
	Tasks []string `args:"true" argsAnywhere:"true"`
}

func (f *Flags) Metadata() map[string]flag.Flag {
	return map[string]flag.Flag{
		"": {
			Desc:    "task runner",
			Arglist: "[TASK]... | list",
		},
	}
}

type indentLogger struct {
	indent string
	debug  bool
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

func (w indentLogger) print(fg color.Attribute, out io.Writer, v ...interface{}) {
	fmt := color.New(fg)
	_, _ = fmt.Fprint(out, w.indent)
	_, _ = fmt.Fprintln(out, v...)
}

func (w indentLogger) fatalln(v ...interface{}) {
	w.print(color.FgHiRed, os.Stderr, v...)
	os.Exit(1)
}

func (w indentLogger) infoln(v ...interface{}) {
	w.print(color.FgHiGreen, os.Stdout, v...)
}

func (w indentLogger) debugln(v ...interface{}) {
	if w.debug {
		w.print(color.FgHiWhite, os.Stdout, v...)
	}
}

func main() {
	var flags Flags
	_ = flag.ParseStruct(&flags)

	if flags.Conf == "" {
		const name = "tash.yaml"
		currDir, _ := filepath.Abs(".")
		for dir := currDir; ; {
			path := filepath.Join(dir, name)
			_, err := os.Stat(path)
			if err == nil || !os.IsNotExist(err) {
				flags.Conf = path
				break
			}
			parent := filepath.Dir(dir)
			if parent == "" || parent == dir {
				flags.Conf = path
				break
			}
			dir = parent
		}
	}
	log := indentLogger{
		debug: flags.Debug,
	}
	content, err := ioutil.ReadFile(flags.Conf)
	if err != nil {
		log.fatalln("read config file failed:", flags.Conf, err)
	}

	var configs Configuration
	err = yaml.Unmarshal(content, &configs)
	if err != nil {
		log.fatalln("parsing config file failed:", flags.Conf, err)
	}
	switch {
	case flags.List.Enable:
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
				if flags.List.Detail {
					fmt.Fprintf(&buf, ": %s", task.Description)
				}
				buf.WriteString("\n")
				buf.WriteTo(os.Stderr)
				buf.Reset()
			}
		}
	default:
		if len(flags.Tasks) == 0 {
			log.fatalln("no tasks to run.")
		}
		currDir, err := os.Getwd()
		if err != nil {
			log.fatalln("get current directory failed:", err)
			return
		}

		for i, name := range flags.Tasks {
			if i > 0 {
				log.infoln() // create new line
			}
			runTaskWithDeps(log, configs, name, currDir)
		}
	}
}

func runTaskWithDeps(log indentLogger, configs Configuration, name, baseDir string) {
	var runned = make(map[string]int)

	var runWithDeps func(log indentLogger, owner, name string)
	runWithDeps = func(log indentLogger, owner, name string) {
		switch runned[name] {
		case 0:
		case 1:
			log.fatalln("task cycle dependency for:", owner)
		case 2:
			return
		}
		runned[name] = 1

		task, ok := configs.searchTask(name)
		if !ok {
			log.fatalln("task not found:", name)
		}
		if len(task.Depends) > 0 {
			log.infoln("Task Deps:", name, task.Depends)
			for i, d := range task.Depends {
				if i > 0 {
					log.infoln()
				}
				runWithDeps(log.addIndent(), owner, d)
			}
		}
		log.infoln("Task:", name)
		runTask(log.addIndent(), configs, name, task, baseDir)

		runned[name] = 2
	}
	runWithDeps(log, name, name)
}

func runTask(log indentLogger, configs Configuration, name string, task Task, baseDir string) {
	workDir := filepath.Join(baseDir, task.WorkDir)
	err := os.Chdir(workDir)
	if err != nil {
		log.fatalln("change working directory failed:", err)
		return
	}

	vars := newVars()
	vars.parseStrings(log, os.Environ())
	vars.parseStrings(log, []string{"WORKDIR=" + workDir, "TASK_NAME=" + name})
	vars.parseEnvs(log, configs.Envs)

	resourceNeedsSync := func(log logger, cpy ActionCopy) bool {
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

		return checkHash(log, cpy.DestPath, cpy.Hash.Alg, cpy.Hash.Sig, fd)
	}
	resourceIsValid := func(log logger, res ActionCopy, path string) bool {
		info, err := os.Stat(path)
		if err != nil {
			log.fatalln("check resource stat failed:", err)
			return false
		}
		if res.Hash.Sig == "" || info.IsDir() {
			return true
		}
		fd, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			log.fatalln("open resource failed:", err)
			return false
		}
		defer fd.Close()

		return checkHash(log, res.SourceUrl, res.Hash.Alg, res.Hash.Sig, fd)
	}
	runCopyAction := func(log indentLogger, cpy ActionCopy, vars *ExpandEnvs) {
		err := vars.expandStrings(&cpy.DestPath, &cpy.SourceUrl)
		if err != nil {
			log.fatalln(err)
		}
		if !resourceNeedsSync(log, cpy) {
			log.debugln("resource reuse.")
			return
		}
		var sourceUrl string
		if strings.Contains(cpy.SourceUrl, ":/") {
			sourceUrl = cpy.SourceUrl
		} else {
			abspath, err := filepath.Abs(cpy.SourceUrl)
			if err != nil {
				log.fatalln("retrieve absolute source path failed:", cpy.SourceUrl, err)
			}
			sourceUrl = "file://" + abspath
		}
		var sourcePath string
		ul, err := url.Parse(sourceUrl)
		if err != nil {
			log.fatalln("couldn't parse source url:", sourceUrl, err)
		}
		switch ul.Scheme {
		case "file":
			if runtime.GOOS == "windows" {
				ul.Path = strings.TrimPrefix(ul.Path, "/")
			}
			sourcePath = ul.Path
		case "http", "https":
			path, err := downloadFile(cpy.SourceUrl)
			if err != nil {
				log.fatalln("download file failed:", cpy.SourceUrl, err)
				return
			}
			sourcePath = path
		default:
			log.fatalln("unsupported source url schema:", ul.Scheme)
		}
		if !resourceIsValid(log, cpy, sourcePath) {
			log.fatalln("resource source invalid:", cpy.SourceUrl)
			return
		}
		err = copyPath(cpy.DestPath, sourcePath)
		if err != nil {
			log.fatalln("resource copy failed:", cpy.SourceUrl, cpy.DestPath, err)
			return
		}
	}
	var runActions func(log indentLogger, vars *ExpandEnvs, a []Action)
	runActionTemplate := func(log indentLogger, action string, vars *ExpandEnvs) {
		actions, ok := configs.searchTemplate(action)
		if !ok {
			log.fatalln("template not found:", action)
			return
		}
		runActions(log.addIndent(), vars, actions)
	}

	runActionCondition := func(log indentLogger, action ActionCondition, vars *ExpandEnvs) {
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
				err := vars.expandStrings(&value, &c.ConditionIf.Compare)
				if err != nil {
					log.fatalln(err)
				}
				return checkCondition(&conditionContext{
					log:           log,
					vars:          vars,
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
				log.fatalln("multiple default cases is not allowed.")
			}
		}
		var defaultCase *ConditionCase
		checkCase := func(i int, c *ConditionCase) bool {
			if c.Default {
				defaultCase = c
			}
			if c.Condition != nil {
				if !validateCondition(c.Condition) {
					log.fatalln("invalid condition case at seq:", i)
				}
				if evalCondition(c.Condition) {
					if i == 0 {
						log.debugln("action condition passed")
					} else {
						log.debugln("action condition case passed at seq:", i)
					}
					runActions(log.addIndentIfDebug(), vars, c.Actions)
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
			log.debugln("action condition run default case")
			runActions(log.addIndentIfDebug(), vars, defaultCase.Actions)
		} else {
			log.debugln("action condition doesn't passed")
		}
	}
	runActionSwitch := func(log indentLogger, action ActionSwitch, vars *ExpandEnvs) {
		{
			var n int
			for i := range action.Cases {
				if action.Cases[i].Default {
					n++
				}
			}
			if n > 1 {
				log.fatalln("multiple default cases is not allowed")
			}
		}
		value := action.Value
		err := vars.expandStrings(&value)
		if err != nil {
			log.fatalln(err)
		}
		var defaultCase SwitchCase
		for _, c := range action.Cases {
			if c.Default {
				defaultCase = c
			}
			if c.Compare != nil {
				compare := *c.Compare
				err := vars.expandStrings(&compare)
				if err != nil {
					log.fatalln(err)
				}
				if checkCondition(&conditionContext{
					log:           log,
					vars:          vars,
					valueOrigin:   action.Value,
					compareOrigin: *c.Compare,
				}, value, action.Operator, compare) {
					log.debugln("action switch case run:", *c.Compare)
					runActions(log.addIndentIfDebug(), vars, c.Actions)
					return
				}
			}
		}
		if defaultCase.Default {
			log.debugln("action switch run default case")
			runActions(log.addIndent(), vars, defaultCase.Actions)
		} else {
			log.debugln("action switch no case matched")
		}
	}
	runActionLoop := func(log indentLogger, action ActionLoop, vars *ExpandEnvs) {
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
				log.fatalln("invalid loop seq:", action.Seq.From, action.Seq.To, step)
			}
			looper = func(fn func(v string)) {
				for i := action.Seq.From; i != action.Seq.To; i += step {
					fn(strconv.Itoa(i))
				}
			}
		case len(action.Array) > 0:
			for i := range action.Array {
				err := vars.expandStrings(&action.Array[i])
				if err != nil {
					log.fatalln(err)
				}
			}
			looper = func(fn func(v string)) {
				for _, v := range action.Array {
					fn(v)
				}
			}
		case action.Split.Value != "":
			err := vars.expandStrings(&action.Split.Value)
			if err != nil {
				log.fatalln(err)
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
			log.fatalln("empty loop block")
			return
		}
		looper(func(v string) {
			vars := vars
			log := log.addIndent()
			if action.Env != "" {
				vars.parseStrings(log, []string{action.Env + "=" + v})

				log.debugln("loop run with env:", action.Env+"="+v)
			}
			runActions(log, vars, action.Actions)
		})
	}
	runActionCmd := func(log indentLogger, action ActionCmd, vars *ExpandEnvs) {
		var fds commandFds
		if action.Stdin != "" {
			fds.Stdin, err = os.OpenFile(action.Stdin, os.O_RDONLY, 0)
			if err != nil {
				log.fatalln("open stdin failed:", err)
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
				log.fatalln("open stdout file failed:", err)
			}
			defer out.Close()
			fds.Stdout = out
		}
		if action.Stderr != "" {
			if action.Stderr == action.Stdout {
				if action.StderrAppend != action.StdoutAppend {
					log.fatalln("couldn't open same stdout/stderr file in different append mode")
				} else {
					action.Stderr = action.Stdout
				}
			} else {
				out, err := openFile(action.Stderr, action.StderrAppend)
				if err != nil {
					log.fatalln("open stderr file failed:", err)
				}
				defer out.Close()
				fds.Stderr = out
			}
		}
		runCommand(log, vars, action.Exec, false, fds)
	}
	runActions = func(log indentLogger, vars *ExpandEnvs, a []Action) {
		for _, a := range a {
			if !reflect.DeepEqual(a.Env, Env{}) {
				log.debugln("Env")
				vars.parseEnvs(log.addIndentIfDebug(), []Env{a.Env})
			}
			if a.Cmd.Exec != "" {
				err := vars.expandStrings(&a.Cmd.Exec, &a.Cmd.Stdin, &a.Cmd.Stdout, &a.Cmd.Stderr)
				if err != nil {
					log.fatalln(err)
				}

				log.infoln("Cmd:", a.Cmd.Exec)
				runActionCmd(log, a.Cmd, vars)
			}
			if a.Copy.DestPath != "" {
				log.infoln("Copy:", a.Copy.SourceUrl, a.Copy.DestPath)
				runCopyAction(log, a.Copy, vars)
			}
			if a.Del != "" {
				log.infoln("Del:", a.Del)
				err = os.RemoveAll(a.Del)
				if err != nil {
					log.fatalln("task action delete failed:", a.Del, err)
				}
			}
			if a.Replace.File != "" && len(a.Replace.Replaces) > 0 {
				log.infoln("Replace:", a.Replace.File)
				err = vars.expandStrings(&a.Replace.File)
				if err != nil {
					log.fatalln(err)
				}
				err = fileReplace(a.Replace.File, a.Replace.Replaces, a.Replace.Regexp)
				if err != nil {
					log.fatalln("task action replace failed:", a.Replace.File, err)
				}
			}
			if a.Chmod.Path != "" {
				log.infoln("Chmod:", a.Chmod.Path)
				err = vars.expandStrings(&a.Chmod.Path)
				if err != nil {
					log.fatalln(err)
				}
				err = os.Chmod(a.Chmod.Path, os.FileMode(a.Chmod.Mode))
				if err != nil {
					log.fatalln("chmod failed:", err)
				}
			}
			if len(a.Chdir.Actions) > 0 {
				log.infoln("Chdir:", a.Chdir.Dir)
				wd, err := os.Getwd()
				if err != nil {
					log.fatalln("get current directory failed:", err)
				}
				err = vars.expandStrings(&a.Chdir.Dir)
				if err != nil {
					log.fatalln(err)
				}
				err = os.Chdir(a.Chdir.Dir)
				if err != nil {
					log.fatalln("chdir failed:", err)
				}
				runActions(log.addIndent(), vars, a.Chdir.Actions)
				err = os.Chdir(wd)
				if err != nil {
					log.fatalln("chdir back failed:", err)
				}
			}
			if a.Template != "" {
				log.infoln("Template:", a.Template)
				runActionTemplate(log, a.Template, vars)
			}
			if len(a.Condition.Cases) > 0 || a.Condition.ConditionCase != nil {
				log.debugln("Condition")
				runActionCondition(log, a.Condition, vars)
			}
			if len(a.Switch.Cases) > 0 {
				log.debugln("Switch")
				runActionSwitch(log, a.Switch, vars)
			}
			if len(a.Loop.Actions) > 0 {
				log.debugln("Loop")
				runActionLoop(log, a.Loop, vars)
			}
		}
	}
	runActions(log, vars, task.Actions)
}
