package syntax

// Expanding:
// almost all strings will be expanded first with environment,
// except:
//	* env name
//	* task work dir
//  * loop seq/times
//  etc..
// the expandable format are:
//	* $ENV_NAME_ALPHA_NUM
//	* ${ENV_NAME_NO_LIMIT [| filter[ arg]...]...}
//	* ${"string literal" [| filter[ arg]...]...}
// uses '\' to avoid escaping, such as '\$', '\$', '\\'
//
// predefined task-specific env:
//    WORKDIR: task initial working directory
//    HOST_OS: host os(GOOS)
//    HOST_ARCH: host arch(GOARCH)
//    TASK_NAME: task name
//
// all path separator '\' on windows have been transformed to slash
// to avoid conflicting with escaping in internal file paths
//
// Imports, Task name, Template name, Env value, File path in Del,Mkdir,Chmod, Replace,Watch
// can all be text block: lines of semicolon separated string
// file path supports zglob, env value should be key=value or key="value"
