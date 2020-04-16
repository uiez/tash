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
	Conf     string   `names:"-c, --conf" usage:"config file, default tash.yaml in current/ancestor directory"`
	Debug    bool     `names:"-d, --debug" usage:"show debug messages"`
	ShowArgs bool     `names:"--list-args" usage:"show task arguments"`
	Tasks    []string `args:"true" argsAnywhere:"true"`
}

func (f *Flags) Metadata() map[string]flag.Flag {
	return map[string]flag.Flag{
		"": {
			Desc:    "task runner",
			Arglist: "[TASK]...",
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
	case len(flags.Tasks) == 0:
		listTasks(&configs, log)
	case flags.ShowArgs:
		listTaskArgs(&configs, flags.Tasks, log)
	default:
		runTasks(&configs, flags.Tasks, log)
	}
}
