package syntax

const (
	BUILTIN_ENV_WORKDIR     = "WORKDIR"
	BUILTIN_ENV_HOST_OS     = "HOST_OS"
	BUILTIN_ENV_HOST_ARCH   = "HOST_ARCH"
	BUILTIN_ENV_TASK_NAME   = "TASK_NAME"
	BUILTIN_ENV_PATHLISTSEP = "PATHLISTSEP"

	// override by every AcionCommand, empty means command failed to start
	BUILTIN_ENV_LAST_COMMAND_PID = "LAST_COMMAND_PID"
)
