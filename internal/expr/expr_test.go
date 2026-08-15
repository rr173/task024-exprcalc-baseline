package expr

import (
	"errors"
	"math"
	"testing"
)

// evalOK 求值表达式并断言成功与期望值（浮点近似比较）。
func evalOK(t *testing.T, s string, vars map[string]float64, want float64) {
	t.Helper()
	got, err := Eval(s, vars)
	if err != nil {
		t.Fatalf("Eval(%q) error: %v", s, err)
	}
	if !approxEqual(got, want) {
		t.Fatalf("Eval(%q) = %g, want %g", s, got, want)
	}
}

// evalErr 求值表达式并断言返回指定哨兵错误。
func evalErr(t *testing.T, s string, vars map[string]float64, sentinel error) {
	t.Helper()
	_, err := Eval(s, vars)
	if err == nil {
		t.Fatalf("Eval(%q) expected %v error, got nil", s, sentinel)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Eval(%q) error = %v, want %v", s, err, sentinel)
	}
}

func approxEqual(a, b float64) bool {
	if a == b {
		return true
	}
	diff := math.Abs(a - b)
	return diff < 1e-9 || diff < 1e-9*math.Max(math.Abs(a), math.Abs(b))
}

func TestArithmetic(t *testing.T) {
	cases := []struct {
		expr string
		want float64
	}{
		{"1 + 2", 3},
		{"1 + 2 * 3", 7},
		{"(1 + 2) * 3", 9},
		{"10 - 3 - 2", 5},      // 左结合
		{"7 / 2", 3.5},        // 浮点除法
		{"10 % 3", 1},
		{"2 + 3 == 5", 1},
		{"3 > 2", 1},
		{"2 > 3", 0},
		{"2 <= 2", 1},
		{"2 >= 3", 0},
		{"3 != 3", 0},
		{"1 && 0", 0},
		{"1 && 1", 1},
		{"0 || 0", 0},
		{"1 || 0", 1},
		{"1.5e3", 1500},
		{"2e-2", 0.02},
		{"12.5", 12.5},
		{"1.", 1},
	}
	for _, c := range cases {
		evalOK(t, c.expr, nil, c.want)
	}
}

func TestPowerSemantics(t *testing.T) {
	cases := []struct {
		expr string
		want float64
	}{
		{"2 ^ 3", 8},
		{"2 ^ 3 ^ 2", 512},  // 右结合
		{"-2 ^ 2", -4},     // 一元负号低于幂
		{"2 ^ -2", 0.25},   // 负指数
		{"(-2) ^ 3", -8},   // 负底数整数次方
		{"(-2) ^ 2", 4},
		{"0 ^ 5", 0},
		{"2 ^ 10", 1024},
		{"pow(2, 10)", 1024},
		{"4 ^ 0.5", 2},
		{"9 ^ 0.5", 3},
	}
	for _, c := range cases {
		evalOK(t, c.expr, nil, c.want)
	}
}

func TestVariables(t *testing.T) {
	vars := map[string]float64{"x": 3, "y": 4, "price": 100, "discount": 0.2}
	evalOK(t, "x * y + 1", vars, 13)
	evalOK(t, "price * (1 - discount) + 5", vars, 85)
	evalOK(t, "x + y", vars, 7)
	evalErr(t, "z + 1", vars, ErrUndefinedVar)
	// 大小写敏感。
	evalErr(t, "X + 1", vars, ErrUndefinedVar)
}

func TestFunctions(t *testing.T) {
	evalOK(t, "max(1, 5, 3)", nil, 5)
	evalOK(t, "max(7)", nil, 7)
	evalOK(t, "abs(-7)", nil, 7)
	evalOK(t, "floor(2.9)", nil, 2)
	evalOK(t, "ceil(2.1)", nil, 3)
	evalOK(t, "round(2.5)", nil, 3)
	evalOK(t, "round(-2.5)", nil, -3)
	evalOK(t, "sqrt(16)", nil, 4)
	evalOK(t, "clamp(15, 0, 10)", nil, 10)
	evalOK(t, "clamp(-5, 0, 10)", nil, 0)
	evalOK(t, "clamp(5, 0, 10)", nil, 5)
	evalOK(t, "if(1, 99, 42)", nil, 99)
	evalOK(t, "if(0, 99, 42)", nil, 42)
}

func TestShortCircuit(t *testing.T) {
	// 未选中分支不求值，不触发除零。
	evalOK(t, "0 && (1/0)", nil, 0)
	evalOK(t, "1 || (1/0)", nil, 1)
	evalOK(t, "if(0, 1/0, 42)", nil, 42)
	evalOK(t, "if(1, 99, 1/0)", nil, 99)
	// 选中分支触发除零则报错。
	evalErr(t, "1 && (1/0)", nil, ErrDivideByZero)
	evalErr(t, "0 || (1/0)", nil, ErrDivideByZero)
}

func TestMathErrors(t *testing.T) {
	evalErr(t, "1 / 0", nil, ErrDivideByZero)
	evalErr(t, "0 / 0", nil, ErrDivideByZero)
	evalErr(t, "5 % 0", nil, ErrModuloByZero)
	evalErr(t, "0 ^ -1", nil, ErrDivideByZero)
	evalErr(t, "sqrt(-1)", nil, ErrDomain)
	evalErr(t, "(-2) ^ 0.5", nil, ErrDomain)
	evalErr(t, "1e308 * 1e308", nil, ErrOverflow)
	evalErr(t, "2 ^ 1024", nil, ErrOverflow)
	// 截断取模符号同被除数。
	evalOK(t, "10 % 3", nil, 1)
}

func TestArgCount(t *testing.T) {
	evalErr(t, "abs(1, 2)", nil, ErrArgCount)
	evalErr(t, "max()", nil, ErrArgCount)
	evalErr(t, "min()", nil, ErrArgCount)
	evalErr(t, "pow(1)", nil, ErrArgCount)
	evalErr(t, "pow(1, 2, 3)", nil, ErrArgCount)
	evalErr(t, "clamp(1, 2)", nil, ErrArgCount)
	evalErr(t, "if(1, 2)", nil, ErrArgCount)
	evalErr(t, "sqrt()", nil, ErrArgCount)
	evalErr(t, "clamp(5, 10, 0)", nil, ErrDomain) // 下界大于上界
}

func TestNamespace(t *testing.T) {
	vars := map[string]float64{"max": 10}
	// 调用位置函数优先：max(1,2) 取内置函数。
	evalOK(t, "max(1, 2)", vars, 2)
	// 非调用位置取变量。
	evalOK(t, "max + 1", vars, 11)
}

func TestSyntaxErrors(t *testing.T) {
	for _, s := range []string{"", "  ", "1 +", "(1 + 2", "1 2", "1 @ 2", "1 + + ", "1 + 2)", "foo(1,", "* 3"} {
		evalErr(t, s, nil, ErrSyntax)
	}
}
