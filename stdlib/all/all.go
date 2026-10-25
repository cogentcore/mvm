// Package all is the convenience aggregator for stdlib bindings: blank-import
// it to get the full set (core + ext).
package all

import (
	_ "github.com/cogentcore/mvm/stdlib/core" // init all bindings
	_ "github.com/cogentcore/mvm/stdlib/ext"  // init all bindings
)
