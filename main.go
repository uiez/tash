package main

import (
	"github.com/cosiner/flag"
)

type Flags struct {
	Conf     string `names:"-c, --conf" usage:"config file, default tash.yaml in current/ancestor directory"`
	SaveConf bool   `names:"-s, --save" usage:"save current config file path to .tashfile" desc:"--conf option should also be present, but it could be omitted in later commands"`
	List     struct {
		Enable bool

		ShowArgs bool     `names:"-w, -with-args" usage:"show task args"`
		Tasks    []string `args:"true" argsAnywhere:"true"`
	} `arglist:"TASK... [OPTION]..."`

	// global command
	Debug    bool     `names:"-d, --debug" usage:"show debug messages"`
	TaskArgs []string `names:"-a, --args" usage:"add task args" desc:"each arg could be multiple semicolon separated key=value pair"`
	Tasks    []string `args:"true" argsAnywhere:"true"`
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

	log := newLogger(flags.Debug)
	configs := parseConfiguration(log, flags.Conf, flags.SaveConf)
	switch {
	default:
		fallthrough
	case flags.List.Enable:
		listTasks(configs, log, flags.List.Tasks, flags.List.ShowArgs)
	case len(flags.Tasks) > 0:
		runTasks(configs, log, flags.Tasks, flags.TaskArgs)
	}
}
