//go:build goscript

package stubs

import (
	"github.com/mvm-sh/mvm/internal/runtype"
)

// FillMethods installs methods on the goscript target, where each method is
// a JavaScript method of the values of its type that calls Handler, so
// native code dispatches interpreted methods directly. No slots are claimed,
// so release is always nil.
func FillMethods(res *runtype.Reservation, methods []Method) (release func(), err error) {
	specs := make([]runtype.MethodSpec, len(methods))
	for i, m := range methods {
		specs[i] = runtype.MethodSpec{
			Name:     m.Name,
			Exported: m.Exported,
			PkgPath:  m.PkgPath,
			Sig:      m.Sig,
			Handler:  m.Handler,
		}
	}
	return nil, res.Fill(specs)
}
