package main

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cosiner/flag"
	"github.com/ghodss/yaml"
	"github.com/zhuah/tash/syntax"
)

type Flags struct {
	Conf string `names:"-c, --conf" usage:"config file, default tash.yaml in current/ancestor directory"`
	List struct {
		Enable bool

		ShowArgs bool     `names:"-a, --args" usage:"show task args"`
		Tasks    []string `args:"true" argsAnywhere:"true"`
	} `arglist:"TASK... [OPTION]..."`

	// global command
	Debug bool     `names:"-d, --debug" usage:"show debug messages"`
	Tasks []string `args:"true" argsAnywhere:"true"`
}

func (f *Flags) Metadata() map[string]flag.Flag {
	return map[string]flag.Flag{
		"": {
			Desc:    "task runner",
			Arglist: "TASK... [OPTION]... | list [TASK]... [OPTION]...",
		},
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
	log := newLogger(flags.Debug)
	content, err := ioutil.ReadFile(flags.Conf)
	if err != nil {
		log.fatalln("read config file failed:", flags.Conf, err)
	}

	var configs syntax.Configuration
	err = yaml.Unmarshal(content, &configs)
	if err != nil {
		log.fatalln("parsing config file failed:", flags.Conf, err)
	}
	switch {
	default:
		fallthrough
	case flags.List.Enable:
		listTasks(&configs, log, flags.List.Tasks, flags.List.ShowArgs)
	case len(flags.Tasks) > 0:
		runTasks(&configs, log, flags.Tasks)
	}
}
