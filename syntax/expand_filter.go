package syntax

// expand filters
const (
	// args: defaultValue
	Ef_string_default = "string.default"
	// args: no args
	Ef_string_trimSpace = "string.trimSpace"
	// args: prefix
	Ef_string_trimPrefix = "string.trimPrefix"
	// args: suffix
	Ef_string_trimSuffix = "string.trimSuffix"
	// nargs: 0: whole string, 1: lower characters after [index], 2:index count, lower [count] characters after [index]
	// index could be negative to iterate from last, begin at -1
	Ef_string_lower = "string.lower"
	// args is same as stringLower, but transform to upper case.
	Ef_string_upper = "string.upper"
	// args is same as stringLower, but returns string inside the range
	Ef_string_slice = "string.slice"
	// split string and return element at index, index can be negative. args: 1: index, sep is ' ', 2: sep index
	Ef_string_at = "string.at"
	// args: [old new]..., do literal replacing
	Ef_string_replace = "string.replace"
	// args: [old new]..., do regexp replacing
	Ef_string_regexpReplace = "string.regexpReplace"
	// sort strings, args: 0: split/join separator is ' ', 1: split/join separator is args[0]
	Ef_string_sort = "string.sort"
	// return files match given pattern, args: 0: join separator is ' ', 1: join separator is args[0]
	Ef_file_glob = "file.glob"
	// args: no args
	Ef_file_abspath = "file.abspath"
	// args: no args
	Ef_file_dirname = "file.dirname"
	// args: no args
	Ef_file_basename = "file.basename"
	// args: no args
	Ef_file_ext   = "file.ext"
	Ef_file_noext = "file.noext"
	// args: no args
	Ef_file_toSlash = "file.toSlash"
	// args: no args
	Ef_file_fromSlash = "file.fromSlash"
	// args: no args
	Ef_file_content = "file.content"
	// args: 0: output as timestamp, 1: output as format, input will be ignored
	Ef_date_now = "date.now"
	// args: format, input should be timestamp
	Ef_date_format = "date.format"
	// args: no args
	Ef_cmd_output = "cmd.output"
)
