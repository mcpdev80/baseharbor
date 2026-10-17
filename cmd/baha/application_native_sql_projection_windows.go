//go:build windows

package main

import "os"

// No public projection ACL authority is established on Windows.
func sqlPublicProjectionOwner(_ os.FileInfo) bool { return false }
