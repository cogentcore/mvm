//go:build mvmnosrc

package interp

// setDefaultFS sets no import file systems: the mvmnosrc build embeds no
// std module and fetches no modules, so interpreted code imports only the
// packages with bindings.
func setDefaultFS(i *Interp) {}
