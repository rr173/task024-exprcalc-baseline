// Package httpapi 提供表达式计算器的 HTTP 接口。
// 服务无内部可变状态，可被多个 goroutine 复用。
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"task024-exprcalc/internal/expr"
)

// ErrBadJSON 表示请求体不是单个合法 JSON 对象。
var ErrBadJSON = errors.New("请求体不是合法的单个 JSON 对象")

// API 是表达式计算器的 HTTP 接口实现。
type API struct{}

// New 创建服务实例。
func New() *API { return &API{} }

// Handler 返回 HTTP 路由。
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /eval", a.eval)
	return mux
}

// decodeJSON 解码单个 JSON 对象，拒绝多段 JSON 与未知字段。
func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return ErrBadJSON
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", ErrBadJSON, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return ErrBadJSON
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type evalRequest struct {
	Expr      string              `json:"expr"`
	Variables map[string]float64 `json:"variables"`
}

type evalResponse struct {
	OK     bool     `json:"ok"`
	Result *float64 `json:"result,omitempty"`
	Error  string   `json:"error,omitempty"`
	Kind   string   `json:"kind,omitempty"`
}

func (a *API) eval(w http.ResponseWriter, r *http.Request) {
	var req evalRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, evalResponse{OK: false, Error: err.Error(), Kind: "bad_request"})
		return
	}

	result, err := expr.Eval(req.Expr, req.Variables)
	if err != nil {
		kind := errorKind(err)
		status := http.StatusBadRequest // 语法错误
		if kind != "syntax" && kind != "bad_request" {
			// 求值过程中的数学错误：返回 200 与 ok=false。
			status = http.StatusOK
		}
		writeJSON(w, status, evalResponse{OK: false, Error: err.Error(), Kind: kind})
		return
	}

	writeJSON(w, http.StatusOK, evalResponse{OK: true, Result: &result})
}

// errorKind 把 expr 包的哨兵错误映射为稳定的类别字符串。
func errorKind(err error) string {
	switch {
	case errors.Is(err, expr.ErrSyntax):
		return "syntax"
	case errors.Is(err, expr.ErrDivideByZero):
		return "divide_by_zero"
	case errors.Is(err, expr.ErrModuloByZero):
		return "modulo_by_zero"
	case errors.Is(err, expr.ErrUndefinedVar):
		return "undefined_variable"
	case errors.Is(err, expr.ErrUnknownFunc):
		return "unknown_function"
	case errors.Is(err, expr.ErrArgCount):
		return "arg_count"
	case errors.Is(err, expr.ErrDomain):
		return "domain_error"
	case errors.Is(err, expr.ErrOverflow):
		return "overflow"
	default:
		return "internal"
	}
}
