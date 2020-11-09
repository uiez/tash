package syntax

// filesystem actions
type fsActions struct {
	// copy resources
	Copy ActionCopy
	// delete file/directory, support glob
	Del ActionDel
	// replace file content
	Replace ActionReplace
	// change file/directory mode
	Chmod ActionChmod
	// create directory and it's parents, ignore if already existed
	Mkdir ActionMkdir
	// watch fs changes, should be last action in a task, it will never returns
	Watch ActionWatch
	// write content to file
	Echo ActionEcho
}

const (
	ResourceHashAlgSha1   = "SHA1"
	ResourceHashAlgMD5    = "MD5"
	ResourceHashAlgSha256 = "SHA256"
)

// resource copy/download
type ActionCopy struct {
	// source url could be file or http/https if contains schema, otherwise it will be treated as file
	// both source and dest could be directory in file mode.
	// doesn't support glob
	SourceUrl string
	// if source is directory, destPath will be removed first, than copy again
	DestPath string
	// Force
	Force string
	// hash checking for file
	Hash struct {
		// hash algorithm, support SHA1, MD5 and SHA256, sha1 by default.
		Alg string
		// hexadecimal string, case insensitive
		Sig string
	}
}

// path delete, support glob
type ActionDel = string

// replace file content
type ActionReplace struct {
	// file path, not directory, support glob
	File string
	// replaces, old,new... pairs
	Replaces []string
	// do regexp replacing
	Regexp bool
}

// change path mode such as 0644 for file, 0755 for directory and executable.
type ActionChmod struct {
	// support glob
	Path string
	Mode uint
}

// create directory and it's parents
type ActionMkdir = string

// write content to file
type ActionEcho struct {
	Content string
	File    string
	Append  bool
}

// watch fs changes
type ActionWatch struct {
	// watch patterns, support glob
	Dirs string
	// file patterns in matched directories, support glob
	Files string

	Actions ActionList
}
