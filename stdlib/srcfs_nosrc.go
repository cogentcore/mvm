//go:build mvmnosrc

package stdlib

// The mvmnosrc build embeds no std module or vendor modules, to keep the
// binary small: interpreted code imports only the packages with bindings.

// EmbeddedStd returns nil: the mvmnosrc build embeds no std module.
func EmbeddedStd() []byte { return nil }

// VendorModule names an embedded third-party module.
type VendorModule struct {
	Path    string
	Version string
	Zip     []byte
}

// EmbeddedVendor returns nil: the mvmnosrc build embeds no vendor modules.
func EmbeddedVendor() []VendorModule { return nil }
