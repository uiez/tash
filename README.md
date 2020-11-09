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
templates:
  build:
    - if:
        check: ${GOOS | "==" "windows"}
        actions:
          env: EXECUTABLE_EXT=.exe
    - cmd:
        exec: go build -ldflags "-w -s" -o tash_${GOOS}_${GOARCH}$EXECUTABLE_EXT

tasks:
  native:
    description: |-
      build native binary
    actions:
      cmd:
        exec: go build -ldflags "-w -s"

  darwin:
    description: |-
      build darwin binary
    args:
      - env: GOARCH
        description: build architecture, amd64 or 386
        default: ${HOST_ARCH}
    actions:
      - env: GOOS=darwin
      - template: build

  linux:
    description: |-
      build linux binary
    args:
      - env: GOARCH
        description: build architecture, amd64 or 386
        default: ${HOST_ARCH}
    actions:
      - env: GOOS=linux
      - template: build

  windows:
    description: |-
      build windows binary
    args:
      - env: GOARCH
        description: build architecture, amd64 or 386
        default: ${HOST_ARCH}
    actions:
      - env: GOOS=windows
      - template: build
  all:
    description: |-
      build darwin,linux,windows binary
    actions:
      task:
        name: darwin; linux; windows

  watch:
    description: |-
      watch fs changes and build native binary
    actions:
      watch:
        dirs: .
        files: "*.go"
        actions:
          task:
            name: native
```

* skia go binding
[tash.yaml](https://github.com/uiez/skia-go/blob/master/tash.yaml)

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