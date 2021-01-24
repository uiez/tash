# Tash

Tash is a yaml based shell for task running.

# Goals
simple, declarative, cross-platform, doesn't depends on unix shell or makefile.

# Install

* With go installed: `go get github.com/uiez/tash`
* Prebuilt binaries: TODO.

# Configuration file location
by default, tash will lookup `tash.yaml` under current/ancestor directories, or user can use `-c/--conf` option.

# Usage
* list tasks: `tash` or `tash list [TASK]... [-a/--args]`
* run tasks: `tash TASK_NAME... [-d/--debug]`
* show help: `tash -h`

# Example
* building tash itself
```YAML
tasks:
  build:
    description: |-
      build native binary
    actions:
      cmd:
        exec: go build -ldflags "-w -s"
```

# Configuration Syntax
defined in [syntax](/syntax) folder.

* [configuration](/syntax/configuration.go)
* actions:
    - [execution context](/syntax/action_context.go)
    - [filesystem](/syntax/action_fs.go)
    - [process](/syntax/action_process.go)
    - [flow control](/syntax/action_flow.go)
        - [comparision operators](/syntax/operator.go)
    - [action reference/reusing](/syntax/action_ref.go)
    
* [built in environment variables](/syntax/builtin_env.go)
* [environment variable expanding](/syntax/expanding.go)
    * [expanding filters](/syntax/expand_filter.go)

# License
MIT.   