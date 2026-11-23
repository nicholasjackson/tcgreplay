package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// hostedPackages are the packages whose log lines, span attributes and
// log record attributes the hosted playmat writes about players.
var hostedPackages = []string{
	".",
	"../../internal/hostauth",
	"../../internal/usergames",
	"../../internal/cacheadmin",
	"../../internal/webui",
	"../../internal/observe",
	"../../internal/cards",
}

// emailLogCalls returns, as file:line, every logger call in src (log.Print*,
// log.Fatal*, log.Panic*, or any Logf or logf) and every span or log
// attribute call (attribute.String and friends, SetAttributes,
// WithAttributes, AddAttributes) whose arguments use a selector ending in
// .Email.
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

// attrFuncs are the OpenTelemetry calls that put a value on a span or a
// log record: the attribute (and otel log) constructors and the methods
// and options that take attributes.
var attrFuncs = map[string]bool{
	"String": true, "StringSlice": true, "KeyValue": true,
	"SetAttributes": true, "WithAttributes": true, "AddAttributes": true,
}

// isLoggerCall reports whether fun is log.Print*, log.Fatal*, log.Panic*,
// a function or method named Logf or logf, or an attribute call: one of
// attrFuncs on the attribute, otellog or log packages, or SetAttributes,
// WithAttributes or AddAttributes on anything.
func isLoggerCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		switch f.Sel.Name {
		case "SetAttributes", "WithAttributes", "AddAttributes":
			return true
		}
		if x, ok := f.X.(*ast.Ident); ok && (x.Name == "attribute" || x.Name == "otellog") && attrFuncs[f.Sel.Name] {
			return true
		}
		if x, ok := f.X.(*ast.Ident); ok && x.Name == "log" {
			if attrFuncs[f.Sel.Name] {
				return true
			}
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

// TestNoLoggerFormatsAnEmail: no logger call, span attribute or log
// attribute in the hosted packages' sources carries an account's email
// address; players are recorded by player ID through the telemetry
// recorder instead.
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

// TestEmailAttributeGuardCatchesALeak: the guard finds an email handed to
// a span or log attribute in each form, and passes player IDs.
func TestEmailAttributeGuardCatchesALeak(t *testing.T) {
	src := `package p
func f() {
	span.SetAttributes(attribute.String("tcgreplay.player", acct.Email))
	tracer.Start(ctx, "x", trace.WithAttributes(attribute.String("who", e.acct.Email)))
	rec.AddAttributes(attribute.String("player", s.account.Email))
	kv := attribute.String("email", acct.Email)
	kvs := attribute.StringSlice("emails", []string{a.Email})
	lv := otellog.String("email", acct.Email)
	span.SetAttributes(attribute.String("tcgreplay.player", rec.ID(acct.Email)))
	span.SetAttributes(attribute.String("tcgreplay.player", playerID))
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "leak.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := emailLogCalls(fset, f)
	lines := map[int]bool{}
	for _, at := range got {
		var line int
		fmt.Sscanf(at[strings.LastIndex(at[:strings.LastIndex(at, ":")], ":")+1:], "%d", &line)
		lines[line] = true
	}
	for _, want := range []int{3, 4, 5, 6, 7, 8} {
		if !lines[want] {
			t.Errorf("guard missed the email attribute on line %d (found %v)", want, got)
		}
	}
	if lines[10] {
		t.Errorf("guard flagged a player ID attribute on line 10 (found %v)", got)
	}
}
