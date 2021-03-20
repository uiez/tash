package syntax

// expand filters
const (
	// dynamically resolve variable
	Ef_var_resolve = "var.resolve"

	// args: defaultValue
	Ef_string_default = "string.default"
	// args: function [function args]
	//	trimSpace:
	//	trimPrefix:
	//	trimSuffix:
	//	quote:
	//	unquote:
	//	upper:
	//	lower:
	//	replace:  [str replace]....
	//	regexpReplace: [regexp replace]....
	Ef_string_transform = "string.transform"
	// nargs: 0: whole string, 1: characters since [index], 2:index count
	// index could be negative to iterate from last, begin at -1
	Ef_string_slice = "string.slice"
	// args: searching, search string
	Ef_string_index = "string.index"
	// args: searching, search string
	Ef_string_lastIndex = "string.lastIndex"

	// number calculating
	// args: operator operand
	Ef_number_calc = "number.calc"

	// args: ok [no]
	Ef_condition_select       = "condition.select"
	Ef_condition_select_alias = "?:"
	// args: [operator [compare]]
	Ef_condition_check       = "condition.check"
	Ef_condition_check_alias = "?"

	// sort strings, args: 0: split/join separator is ' ', 1: split/join separator is args[0]
	Ef_array_sort = "array.sort"
	// sort strings as number, args: same as array.sort
	Ef_array_numSort = "array.numsort"
	// reverse array elements, args: same as array.sort
	Ef_array_reverse = "array.reverse"
	// filter array elements, args: 1: operator, 2: operator compare, 3: operator compare array-separator
	Ef_array_filter = "array.filter"

	// split string and return element at index, index can be negative.
	// args: 1: index
	Ef_array_get = "array.get"
	// split string and return index of element, index can not be negative.
	// args: 1: element
	Ef_array_index = "array.index"
	// args: 1: element
	Ef_array_has = "array.has"

	// split string, calculate array slice and join
	// args: 1: index
	//       2: index count
	//       3:  separator index count
	Ef_array_slice = "array.slice"
	// reset array separator, args: 1: new separator, 2: old new
	Ef_array_separator = "array.separator"

	// split string and return element by key, args: 1: key, 2: key, array separator
	Ef_map_get = "map.get"
	// split string and return keys, args: 0: no args, 1: array separator
	Ef_map_keys = "map.keys"
	// split string and return values, args: 0: no args, 1: array separator
	Ef_map_values = "map.values"

	// get content from json, args: key, nested by '.'
	Ef_json_get = "json.get"

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
	// args: 0: no args, 1: working directory
	Ef_cmd_output = "cmd.output"
)
