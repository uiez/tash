package syntax

// operators in condition and switch
// there is a sugar that put a Op_bool_not before actual operator to do not checking.
const (
	Op_bool_not                  = "bool.not"
	Op_bool_true                 = "bool.true"
	Op_string_greaterThan        = "string.greaterThan"
	Op_string_greaterThanOrEqual = "string.greaterThanOrEqual"
	Op_string_equal              = "string.equal"
	Op_string_notEqual           = "string.notEqual"
	Op_string_lessThanOrEqual    = "string.lessThanOrEqual"
	Op_string_lessThan           = "string.lessThan"
	Op_string_notEmpty           = "string.notEmpty"
	Op_string_empty              = "string.empty"
	Op_string_regexp             = "string.regexp"
	Op_number_greaterThan        = "number.greaterThan"
	Op_number_greaterThanOrEqual = "number.greaterThanOrEqual"
	Op_number_equal              = "number.equal"
	Op_number_notEqual           = "number.notEqual"
	Op_number_lessThanOrEqual    = "number.lessThanOrEqual"
	Op_number_lessThan           = "number.lessThan"
	Op_env_defined               = "env.defined"
	Op_file_newerThan            = "file.newerThan"
	Op_file_olderThan            = "file.olderThan"
	Op_file_exist                = "file.exist"
	Op_file_blockDevice          = "file.blockDevice"
	Op_file_charDevice           = "file.charDevice"
	Op_file_dir                  = "file.dir"
	Op_file_regular              = "file.regular"
	Op_file_setgid               = "file.setgid"
	Op_file_symlink              = "file.symlink"
	Op_file_sticky               = "file.sticky"
	Op_file_namedPipe            = "file.namedPipe"
	Op_file_notEmpty             = "file.notEmpty"
	Op_file_socket               = "file.socket"
	Op_file_setuid               = "file.setuid"
)

var OperatorAlias = map[string]string{
	"?":    Op_bool_true,
	"!":    Op_bool_not,
	"not":  Op_bool_not,
	">":    Op_string_greaterThan,
	">=":   Op_string_greaterThanOrEqual,
	"==":   Op_string_equal,
	"!=":   Op_string_notEqual,
	"<=":   Op_string_lessThanOrEqual,
	"<":    Op_string_lessThan,
	"-n":   Op_string_notEmpty,
	"-z":   Op_string_empty,
	"=~":   Op_string_regexp,
	"-gt":  Op_number_greaterThan,
	"-ge":  Op_number_greaterThanOrEqual,
	"-eq":  Op_number_equal,
	"-ne":  Op_number_notEqual,
	"-le":  Op_number_lessThanOrEqual,
	"-lt":  Op_number_lessThan,
	"-env": Op_env_defined,
	"-nt":  Op_file_newerThan,
	"-ot":  Op_file_olderThan,
	"-a":   Op_file_exist,
	"-e":   Op_file_exist,
	"-b":   Op_file_blockDevice,
	"-c":   Op_file_charDevice,
	"-d":   Op_file_dir,
	"-f":   Op_file_regular,
	"-g":   Op_file_setgid,
	"-h":   Op_file_symlink,
	"-L":   Op_file_symlink,
	"-k":   Op_file_sticky,
	"-p":   Op_file_namedPipe,
	"-s":   Op_file_notEmpty,
	"-S":   Op_file_socket,
	"-u":   Op_file_setuid,
}
