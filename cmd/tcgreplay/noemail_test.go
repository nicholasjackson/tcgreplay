package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// hostedPackages are the packages whose log lines the hosted playmat
// writes about players.
var hostedPackages = []string{
	".",
	"../../internal/hostauth",
	"../../internal/usergames",
	"../../internal/cacheadmin",
	"../../internal/webui",
}

// emailLogCalls returns, as file:line, every logger call in src (log.Print*,
// log.Fatal*, log.Panic*, or any Logf or logf) whose arguments use a
// selector ending in .Email.
func emailLogCalls(fset *token.FileSet, f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isLoggerCall(call.Fun) {
			return true
		}
		for _, arg := range call.Args {
			ast.Inspect(arg, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Email" {
					out = append(out, fset.Position(call.Pos()).String())
					return false
				}
				return true
			})
		}
		return true
	})
	return out
}

// isLoggerCall reports whether fun is log.Print*, log.Fatal*, log.Panic*,
// or a function or method named Logf or logf.
func isLoggerCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok && x.Name == "log" {
			for _, p := range []string{"Print", "Fatal", "Panic"} {
				if strings.HasPrefix(f.Sel.Name, p) {
					return true
				}
			}
		}
		return f.Sel.Name == "Logf" || f.Sel.Name == "logf"
	case *ast.Ident:
		return f.Name == "Logf" || f.Name == "logf"
	}
	return false
}

// TestNoLoggerFormatsAnEmail: no logger call in the hosted packages'
// sources formats an account's email address; players are recorded by
// player ID through the telemetry recorder instead.
func TestNoLoggerFormatsAnEmail(t *testing.T) {
	fset := token.NewFileSet()
	files := 0
	for _, dir := range hostedPackages {
		paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files++
			for _, at := range emailLogCalls(fset, f) {
				t.Errorf("%s: a logger call formats an email; record the event through the telemetry recorder instead", at)
			}
		}
	}
	if files < 10 {
		t.Fatalf("only %d source files scanned; the package paths are wrong", files)
	}
}

// TestEmailLogGuardCatchesALeak: the guard finds each way of logging an
// email, and passes the recorder's own calls.
func TestEmailLogGuardCatchesALeak(t *testing.T) {
	src := `package p
func f() {
	log.Printf("signed in %s", acct.Email)
	log.Println(fmt.Sprintf("x %s", e.acct.Email))
	s.logf("cache admin: %s", s.account.Email)
	r.cfg.Recorder.Record(telemetry.Event{Name: telemetry.SignIn, Email: acct.Email})
	log.Printf("usergames: %d accounts", n)
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "leak.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := emailLogCalls(fset, f); len(got) != 3 {
		t.Errorf("guard found %v, want the 3 logging calls on lines 3-5", got)
	}
}
