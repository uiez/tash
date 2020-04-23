package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/zhuah/tash/syntax"
)

type Configuration struct {
	// defines global environment variables.
	Envs []syntax.Env
	// defines templates(action list) can be referenced from tasks.
	// the key is template name
	Templates map[string][]syntax.Action

	// defines tasks
	// the key is task name
	Tasks map[string]syntax.Task
}

func parseConfiguration(log logger, conf string, saveConf bool) *Configuration {
	if conf == "" {
		var recorded bool
		conf, recorded = lookupConfigurationPath()
		if conf == "" {
			log.fatalln("couldn't find configuration file in current or parent directories")
		}
		if recorded {
			log.infoln(fmt.Sprintf("use config file path '%s' record in %s", conf, recordFile))
		} else {
			log.infoln(fmt.Sprintf("use config file path '%s'", conf))
		}
	} else {
		if saveConf {
			log.infoln("saving config file path to .tashfile")
			err := ioutil.WriteFile(recordFile, []byte(conf), 0644)
			if err != nil {
				log.fatalln("save config file path failed:", err)
			}
		}
	}
	c := &Configuration{
		Envs:      nil,
		Templates: make(map[string][]syntax.Action),
		Tasks:     make(map[string]syntax.Task),
	}
	c.buildFrom(log, conf)
	return c
}

const recordFile = ".tashfile"

func lookupConfigurationPath() (path string, isRecorded bool) {
	content, err := ioutil.ReadFile(recordFile)
	if err == nil {
		path := string(content)
		if path != "" {
			stat, err := os.Stat(path)
			if err == nil && !stat.IsDir() {
				return path, true
			}
		}
	}

	const defaultConf = "tash.yaml"
	currDir, _ := filepath.Abs(".")
	for dir := currDir; ; {
		path := filepath.Join(dir, defaultConf)
		_, err := os.Stat(path)
		if err == nil || !os.IsNotExist(err) {
			return path, false
		}
		parent := filepath.Dir(dir)
		if parent == "" || parent == dir {
			return path, false
		}
		dir = parent
	}
}

func (c *Configuration) importFile(log logger, path string) {
	switch filepath.Ext(path) {
	case ".env":
		content, err := ioutil.ReadFile(path)
		if err != nil {
			log.fatalln("read env file content failed:", err)
			return
		}
		c.Envs = append(c.Envs, syntax.Env{
			Value: string(content),
		})
	default:
		fallthrough
	case ".yaml", ".yml":
		c.buildFrom(log, path)
	}
}

func (c *Configuration) buildFrom(log logger, path string) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		log.fatalln("read config file failed:", path, err)
		return
	}

	var configs syntax.Configuration
	err = yaml.Unmarshal(content, &configs)
	if err != nil {
		log.fatalln("parsing config file failed:", path, err)
		return
	}

	if len(configs.Imports) > 0 {
		dir := filepath.Dir(path)
		for _, imp := range configs.Imports {
			if !filepath.IsAbs(imp) {
				imp = filepath.Join(dir, imp)
			}

			c.importFile(log, imp)
		}
	}
	c.Envs = append(c.Envs, configs.Envs...)
	for name, actions := range configs.Templates {
		_, has := c.Templates[name]
		if has {
			log.fatalln("duplicated template definition:", name)
		}
		c.Templates[name] = actions
	}
	for name, task := range configs.Tasks {
		_, has := c.Tasks[name]
		if has {
			log.fatalln("duplicated task definition:", name)
		}
		c.Tasks[name] = task
	}
}
