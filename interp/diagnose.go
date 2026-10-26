package interp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cogentcore/mvm/vm"
)

// Diagnostic is the source position and message of an error of Eval,
// for tools that show where the error is in the source code.
type Diagnostic struct {
	// Source is the name of the source, as given to Eval.
	Source string

	// Line and Col are the 1-based line and column (in bytes) of the
	// position in the source.
	Line, Col int

	// Msg is the message of the error, without its position and the
	// snippet of the source that the error message has.
	Msg string

	// Panic is whether the error is a panic of the running code, rather
	// than an error of compiling it.
	Panic bool
}

// Diagnose returns the source position and message of the given error,
// returned by Eval, and whether it has a position in the source.
func (i *Interp) Diagnose(err error) (Diagnostic, bool) {
	var d Diagnostic
	var pos int
	var pe *vm.PanicError
	var sp interface{ SourcePos() int }
	var ep interface{ ErrPos() int }
	switch {
	case errors.As(err, &pe):
		pos, d.Panic = int(pe.Pos), true
		d.Msg = "panic: " + fmt.Sprint(pe.Raw)
	case errors.As(err, &sp):
		pos = sp.SourcePos()
		d.Msg = errorMessage(sp.(error))
	case errors.As(err, &ep):
		pos = ep.ErrPos()
		d.Msg = errorMessage(ep.(error))
	default:
		return d, false
	}
	d.Source, d.Line, d.Col = i.Sources.Resolve(pos)
	if d.Source == "" {
		return d, false
	}
	if loc := i.Sources.FormatPos(pos) + ": "; !d.Panic {
		d.Msg = strings.TrimPrefix(d.Msg, loc)
	}
	return d, true
}

// errorMessage returns the first line of the message of the given error,
// which has no snippet of the source.
func errorMessage(err error) string {
	msg, _, _ := strings.Cut(err.Error(), "\n")
	return msg
}
