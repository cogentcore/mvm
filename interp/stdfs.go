//go:build !mvmnosrc

package interp

import "github.com/cogentcore/mvm/stdlib/stdmod"

// setDefaultFS sets the default import file systems of i: the embedded std
// module for stdlib imports, and the offline module cache for others.
func setDefaultFS(i *Interp) {
	i.SetStdlibFS(stdmod.DefaultFS())
	i.SetRemoteFS(stdmod.DefaultRemoteFS())
}
