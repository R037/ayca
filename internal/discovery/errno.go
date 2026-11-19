package discovery

import "syscall"

// syscallENOTDIR is returned when a path component is a file (e.g. looking
// for .claude/CLAUDE.md where .claude is a file). Treated like "not found".
var syscallENOTDIR error = syscall.ENOTDIR
