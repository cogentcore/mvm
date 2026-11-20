//go:build goscript

package wordabi

// goscriptTarget reports a goscript (JavaScript) build, where methods are
// never dispatched through native stubs, so no signature has a word shape.
const goscriptTarget = true
