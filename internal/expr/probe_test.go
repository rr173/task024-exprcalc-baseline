package expr

import (
	"errors"
	"testing"
)

// TestProbeMinMultipleArgs verifies that the minimum of several values
// is correctly identified when values are not in sorted order.
func TestProbeMinMultipleArgs(t *testing.T) {
	// min(9, 2, 7, 1, 5) should return 1, not the largest value.
	evalOK(t, "min(9, 2, 7, 1, 5)", nil, 1)
	evalOK(t, "min(3, 1)", nil, 1)
	evalOK(t, "min(100, 50, 200, 10, 99)", nil, 10)
}

// TestProbeUnknownFunctionError verifies that calling a function name
// not in the built-in registry produces a proper error, not a crash.
func TestProbeUnknownFunctionError(t *testing.T) {
	cases := []string{"foo(1)", "bar(1,2)", "unknown()"}
	for _, s := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Eval(%q) panicked: %v", s, r)
				}
			}()
			_, err := Eval(s, nil)
			if err == nil {
				t.Errorf("Eval(%q) expected error for unknown function, got nil", s)
			}
			if !errors.Is(err, ErrUnknownFunc) {
				t.Errorf("Eval(%q) error = %v, want ErrUnknownFunc", s, err)
			}
		}()
	}
}

// TestProbeZeroPowZeroConvention verifies the mathematical convention
// that 0 raised to the power 0 equals 1.
func TestProbeZeroPowZeroConvention(t *testing.T) {
	evalOK(t, "0 ^ 0", nil, 1)
	evalOK(t, "pow(0, 0)", nil, 1)
}

// TestProbeDotPrefixLiteral verifies that numeric literals starting
// with a decimal point (like .5 or .123) are correctly parsed.
func TestProbeDotPrefixLiteral(t *testing.T) {
	evalOK(t, ".5 + .5", nil, 1)
	evalOK(t, ".25 * 4", nil, 1)
	evalOK(t, ".1 + .2 + .7", nil, 1)
}

// TestProbeModuloSignConvention verifies that the modulo operator
// follows truncated division semantics where the result sign matches
// the dividend sign.
func TestProbeModuloSignConvention(t *testing.T) {
	// -5 % 3 should be -2 (sign matches dividend -5)
	evalOK(t, "-5 % 3", nil, -2)
	// 5 % -3 should be 2 (sign matches dividend 5)
	evalOK(t, "5 % -3", nil, 2)
	// -7 % 4 should be -3
	evalOK(t, "-7 % 4", nil, -3)
}
