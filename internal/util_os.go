package internal

import "os"

// osHostname is a small shim for testability.
func OsHostname() (string, error) { return os.Hostname() }
