// Package selfcheck 提供无需外部依赖的自检：通过 httptest 启动真实 HTTP
// 服务，覆盖计算端点与各边界约束。成功返回 0，任一失败返回 1。
package selfcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"task024-exprcalc/internal/httpapi"
)

// Run 执行自检并返回退出码。
func Run() int {
	passed, failed := 0, 0
	check := func(name string, fn func() error) {
		if err := fn(); err != nil {
			failed++
			fmt.Printf("FAIL %-36s %v\n", name, err)
		} else {
			passed++
			fmt.Printf("PASS %s\n", name)
		}
	}

	srv := httptest.NewServer(httpapi.New().Handler())
	defer srv.Close()

	do := func(method, path, body string) (*http.Response, []byte, error) {
		var r io.Reader
		if body != "" {
			r = bytes.NewReader([]byte(body))
		}
		req, err := http.NewRequest(method, srv.URL+path, r)
		if err != nil {
			return nil, nil, err
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, data, readErr
	}
	marshal := func(m map[string]any) string {
		b, _ := json.Marshal(m)
		return string(b)
	}

	// eval 端点封装：返回状态码、是否成功、结果、错误类别与错误信息。
	eval := func(exprStr string, vars map[string]any) (status int, ok bool, result *float64, kind, errStr string, err error) {
		body := marshal(map[string]any{"expr": exprStr, "variables": vars})
		resp, data, err := do(http.MethodPost, "/eval", body)
		if err != nil {
			return 0, false, nil, "", "", err
		}
		var out struct {
			OK     bool     `json:"ok"`
			Result *float64 `json:"result"`
			Kind   string   `json:"kind"`
			Error  string   `json:"error"`
		}
		_ = json.Unmarshal(data, &out)
		return resp.StatusCode, out.OK, out.Result, out.Kind, out.Error, nil
	}

	// assertOK 断言求值成功且结果等于期望（浮点近似）。
	assertOK := func(name, exprStr string, vars map[string]any, want float64) {
		check(name, func() error {
			status, ok, result, kind, errStr, err := eval(exprStr, vars)
			if err != nil {
				return err
			}
			if status != http.StatusOK {
				return fmt.Errorf("status=%d err=%s", status, errStr)
			}
			if !ok {
				return fmt.Errorf("ok=false kind=%s err=%s", kind, errStr)
			}
			if result == nil {
				return fmt.Errorf("result is nil")
			}
			if !approxEqual(*result, want) {
				return fmt.Errorf("result=%g want %g", *result, want)
			}
			return nil
		})
	}

	// assertKind 断言求值失败（语义错误，200 ok=false）并匹配类别。
	assertKind := func(name, exprStr string, vars map[string]any, wantKind string) {
		check(name, func() error {
			status, ok, result, kind, errStr, err := eval(exprStr, vars)
			if err != nil {
				return err
			}
			if status != http.StatusOK {
				return fmt.Errorf("status=%d want 200 (语义错误)", status)
			}
			if ok {
				return fmt.Errorf("ok=true want false")
			}
			if result != nil {
				return fmt.Errorf("result=%v want nil on error", result)
			}
			if kind != wantKind {
				return fmt.Errorf("kind=%q want %q (err=%s)", kind, wantKind, errStr)
			}
			return nil
		})
	}

	// assertStatus 断言 HTTP 状态码与类别（用于 400）。
	assertStatus := func(name, exprStr string, vars map[string]any, wantStatus int, wantKind string) {
		check(name, func() error {
			body := marshal(map[string]any{"expr": exprStr, "variables": vars})
			resp, data, err := do(http.MethodPost, "/eval", body)
			if err != nil {
				return err
			}
			if resp.StatusCode != wantStatus {
				return fmt.Errorf("status=%d want %d body=%s", resp.StatusCode, wantStatus, data)
			}
			var out struct {
				OK   bool   `json:"ok"`
				Kind string `json:"kind"`
			}
			_ = json.Unmarshal(data, &out)
			if wantKind != "" && out.Kind != wantKind {
				return fmt.Errorf("kind=%q want %q", out.Kind, wantKind)
			}
			return nil
		})
	}

	// ---- 健康检查 ----
	check("健康检查", func() error {
		resp, _, err := do(http.MethodGet, "/healthz", "")
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	// ---- 基础算术与优先级 ----
	assertOK("基础算术与优先级", "1 + 2 * 3", nil, 7)
	assertOK("括号分组", "(1 + 2) * 3", nil, 9)
	assertOK("左结合减法", "10 - 3 - 2", nil, 5)
	assertOK("浮点除法", "7 / 2", nil, 3.5)
	assertOK("科学计数法", "1.5e3 + 2e-2", nil, 1500.02)

	// ---- 幂运算语义（约束2）----
	assertOK("幂右结合 2^3^2=512", "2 ^ 3 ^ 2", nil, 512)
	assertOK("一元负号低于幂 -2^2=-4", "-2 ^ 2", nil, -4)
	assertOK("负指数 2^-2=0.25", "2 ^ -2", nil, 0.25)
	assertOK("负底数整数次方 (-2)^3=-8", "(-2) ^ 3", nil, -8)
	assertOK("0^0=1 约定", "0 ^ 0", nil, 1)
	assertOK("0^5=0", "0 ^ 5", nil, 0)
	assertOK("pow 与 ^ 同义", "pow(2, 10)", nil, 1024)
	assertOK("平方根 sqrt(16)=4", "sqrt(16)", nil, 4)

	// ---- 取模与除零（约束1）----
	assertOK("截断取模 -5%3=-2", "-5 % 3", nil, -2)
	assertOK("取模符号同被除数 5%-3=2", "5 % -3", nil, 2)
	assertOK("取模 10%3=1", "10 % 3", nil, 1)
	assertKind("除零报错", "1 / 0", nil, "divide_by_zero")
	assertKind("零除零报错", "0 / 0", nil, "divide_by_zero")
	assertKind("取模零报错", "5 % 0", nil, "modulo_by_zero")
	assertKind("0 的负数次方报除零", "0 ^ -1", nil, "divide_by_zero")

	// ---- 定义域与溢出（约束1、2）----
	assertKind("sqrt 负数定义域错", "sqrt(-1)", nil, "domain_error")
	assertKind("负底数非整数次方定义域错", "(-2) ^ 0.5", nil, "domain_error")
	assertKind("乘法溢出", "1e308 * 1e308", nil, "overflow")
	assertKind("幂溢出 2^1024", "2 ^ 1024", nil, "overflow")

	// ---- 变量与命名空间（约束4）----
	vars := map[string]any{"x": 3, "y": 4, "price": 100, "discount": 0.2}
	assertOK("变量绑定求值", "x * y + 1", vars, 13)
	assertOK("变量复合表达式", "price * (1 - discount) + 5", vars, 85)
	assertKind("未定义变量报错", "z + 1", nil, "undefined_variable")
	assertKind("变量大小写敏感", "X + 1", map[string]any{"x": 1}, "undefined_variable")
	// 命名空间：max 既是变量名又是函数名。
	nsVars := map[string]any{"max": 10}
	assertOK("调用位置函数优先 max(1,2)=2", "max(1, 2)", nsVars, 2)
	assertOK("非调用位置取变量 max+1=11", "max + 1", nsVars, 11)
	assertKind("未知函数报错", "foo(1)", nil, "unknown_function")

	// ---- 函数与元数（约束3）----
	assertOK("max 变参", "max(1, 5, 3, 9, 2)", nil, 9)
	assertOK("min 变参", "min(1, 5, 3)", nil, 1)
	assertOK("max 单参数", "max(7)", nil, 7)
	assertOK("abs", "abs(-7)", nil, 7)
	assertOK("floor", "floor(2.9)", nil, 2)
	assertOK("ceil", "ceil(2.1)", nil, 3)
	assertOK("round 正", "round(2.5)", nil, 3)
	assertOK("round 负", "round(-2.5)", nil, -3)
	assertOK("clamp 上界", "clamp(15, 0, 10)", nil, 10)
	assertOK("clamp 下界", "clamp(-5, 0, 10)", nil, 0)
	assertOK("clamp 区间内", "clamp(5, 0, 10)", nil, 5)
	assertOK("if 真", "if(1, 99, 42)", nil, 99)
	assertOK("if 假", "if(0, 99, 42)", nil, 42)
	assertKind("abs 元数错误", "abs(1, 2)", nil, "arg_count")
	assertKind("max 零参数错误", "max()", nil, "arg_count")
	assertKind("pow 元数错误", "pow(1)", nil, "arg_count")
	assertKind("clamp 元数错误", "clamp(1, 2)", nil, "arg_count")
	assertKind("if 元数错误", "if(1, 2)", nil, "arg_count")
	assertKind("clamp 下界大于上界", "clamp(5, 10, 0)", nil, "domain_error")

	// ---- 短路求值（约束3）----
	assertOK("短路 && 不触发除零", "0 && (1/0)", nil, 0)
	assertOK("短路 || 不触发除零", "1 || (1/0)", nil, 1)
	assertOK("if 短路 假分支", "if(0, 1/0, 42)", nil, 42)
	assertOK("if 短路 真分支", "if(1, 99, 1/0)", nil, 99)
	assertKind("&& 选中分支触发除零", "1 && (1/0)", nil, "divide_by_zero")
	assertKind("|| 选中分支触发除零", "0 || (1/0)", nil, "divide_by_zero")

	// ---- 比较与逻辑 ----
	assertOK("3>2=1", "3 > 2", nil, 1)
	assertOK("2>3=0", "2 > 3", nil, 0)
	assertOK("3==3=1", "3 == 3", nil, 1)
	assertOK("3!=3=0", "3 != 3", nil, 0)
	assertOK("2<=2=1", "2 <= 2", nil, 1)
	assertOK("2>=3=0", "2 >= 3", nil, 0)
	assertOK("1&&0=0", "1 && 0", nil, 0)
	assertOK("1||0=1", "1 || 0", nil, 1)

	// ---- 语法错误（400）----
	assertStatus("空表达式 400", "", nil, http.StatusBadRequest, "syntax")
	assertStatus("纯空白 400", "   ", nil, http.StatusBadRequest, "syntax")
	assertStatus("尾随运算符 400", "1 +", nil, http.StatusBadRequest, "syntax")
	assertStatus("未闭合括号 400", "(1 + 2", nil, http.StatusBadRequest, "syntax")
	assertStatus("多余记号 400", "1 2", nil, http.StatusBadRequest, "syntax")
	assertStatus("非法字符 400", "1 @ 2", nil, http.StatusBadRequest, "syntax")
	assertStatus("多余右括号 400", "1 + 2)", nil, http.StatusBadRequest, "syntax")
	assertStatus("函数缺右括号 400", "max(1, 2", nil, http.StatusBadRequest, "syntax")

	// ---- 请求格式校验（400）----
	check("非法 JSON 被拒(400)", func() error {
		resp, _, err := do(http.MethodPost, "/eval", "{not json")
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", resp.StatusCode)
		}
		return nil
	})
	check("多段 JSON 被拒(400)", func() error {
		b1 := marshal(map[string]any{"expr": "1+1", "variables": map[string]any{}})
		b2 := marshal(map[string]any{"expr": "2+2", "variables": map[string]any{}})
		resp, _, err := do(http.MethodPost, "/eval", b1+b2)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", resp.StatusCode)
		}
		return nil
	})
	check("未知字段被拒(400)", func() error {
		resp, _, err := do(http.MethodPost, "/eval", marshal(map[string]any{"expr": "1+1", "extra": 1}))
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", resp.StatusCode)
		}
		return nil
	})
	check("expr 非字符串被拒(400)", func() error {
		resp, data, err := do(http.MethodPost, "/eval", marshal(map[string]any{"expr": 123}))
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400 body=%s", resp.StatusCode, data)
		}
		return nil
	})
	check("variables 非对象被拒(400)", func() error {
		resp, _, err := do(http.MethodPost, "/eval", marshal(map[string]any{"expr": "1+1", "variables": "nope"}))
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", resp.StatusCode)
		}
		return nil
	})
	check("变量值非数字被拒(400)", func() error {
		resp, _, err := do(http.MethodPost, "/eval", `{"expr":"x+1","variables":{"x":"str"}}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", resp.StatusCode)
		}
		return nil
	})

	// ---- 结果为 0 仍返回 result 字段 ----
	check("结果为 0 仍返回 result", func() error {
		resp, data, err := do(http.MethodPost, "/eval", marshal(map[string]any{"expr": "5 - 5"}))
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		if !strings.Contains(string(data), `"result":0`) {
			return fmt.Errorf("expected result:0 in response, got: %s", data)
		}
		return nil
	})

	fmt.Printf("\n%d passed, %d failed\n", passed, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func approxEqual(a, b float64) bool {
	if a == b {
		return true
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9 || diff < 1e-9*maxf(a, b)
}

func maxf(a, b float64) float64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if a > b {
		return a
	}
	return b
}
