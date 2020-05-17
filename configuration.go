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

func parseConfiguration(log indentLogger, conf string, saveConf bool) *Configuration {
	currDir, _ := os.Getwd()

	if conf == "" {
		var recorded bool
		conf, recorded = lookupConfigurationPath(currDir)
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
	c.buildFrom(log, currDir, conf)
	return c
}

const recordFile = ".tashfile"

func lookupConfigurationPath(currDir string) (path string, isRecorded bool) {
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
	for dir := currDir; ; {
		path := filepath.Join(dir, defaultConf)
		stat, err := os.Stat(path)
		if err == nil && !stat.IsDir() {
			relpath, err := filepath.Rel(currDir, path)
			if err != nil {
				return path, false
			}
			return relpath, false
		}
		parent := filepath.Dir(dir)
		if parent == "" || parent == dir {
			return "", false
		}
		dir = parent
	}
}

func (c *Configuration) importPath(log indentLogger, baseDir, path string) {
	var relpath string
	{
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			log.fatalln("get file abs path failed:", err)
			return
		}

		p, err := filepath.Rel(baseDir, path)
		if err == nil {
			relpath = p
		} else {
			relpath = path
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.warnln("file imported doesn't existed, skip:", relpath)
		} else {
			log.warnln("retrieve file status failed:", relpath, err)
		}
		return
	}
	if info.IsDir() {
		log.warnln("imported path is directory, skipped, file status failed", relpath)
		return
	}

	switch filepath.Ext(path) {
	case ".env":
		log.debugln("import environment file:", relpath)
		content, err := ioutil.ReadFile(path)
		if err != nil {
			log.fatalln("read env file content failed:", err)
			return
		}
		c.Envs = append(c.Envs, syntax.Env{
			Value: string(content),
		})
	default:
		log.debugln("ignore file:", relpath)
	case ".yaml", ".yml":
		log.debugln("import tash config file:", relpath)
		c.buildFrom(log.addIndent(), baseDir, path)
	}
}

func (c *Configuration) buildFrom(log indentLogger, baseDir, path string) {
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

	if configs.Imports != "" {
		dir := filepath.Dir(path)
		err = runInDir(dir, func() error {
			matched, err := splitBlocksAndGlobPath(configs.Imports, true)
			if err != nil {
				return fmt.Errorf("glob path failed: %w", err)
			}
			for _, m := range matched {
				c.importPath(log, baseDir, m)
			}
			return nil
		})
		if err != nil {
			log.fatalln("import files failed:", err)
			return
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
