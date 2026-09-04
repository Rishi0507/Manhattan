package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"
)

// The invariant this test exists to enforce, and it has been broken twice.
//
// deriveDocs fills a Derived struct field by field, and a field that reads
// another Derived field depends on the order of statements. Nothing about that
// dependency is visible: the code compiles, the template renders, every test
// passes, and the number is silently zero.
//
// It shipped a business case worth nothing per month. It was fixed. A guard was
// added for the specific fields involved. Then a new field, BreakEvenDefectPct,
// was written to read ExtraWorkINR six lines above the comment warning against
// exactly that, and rendered "below a report defect rate of about 0.0 per cent,
// checking costs more than it saves" into the README, which reads as
// self-refuting.
//
// Guarding the fields was the wrong fix. This guards the CLASS: walk the
// function in statement order, and fail if any Derived field is read before it
// has been assigned. A third occurrence is now a build failure rather than a
// discovery by a reviewer.
func TestDerivedFieldsAreNeverReadBeforeAssignment(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "docs.go", nil, 0)
	if err != nil {
		t.Fatalf("parse docs.go: %v", err)
	}

	fn := findFunc(file, "deriveDocs")
	if fn == nil {
		t.Fatal("deriveDocs not found; this test guards it and must be updated with it")
	}

	assigned := map[string]bool{}
	type violation struct {
		field string
		pos   token.Position
	}
	var bad []violation

	// Walk the body in source order. For each statement, read first and then
	// record what it assigns, so a self-referential append is fine and a read
	// of something assigned later is not.
	for _, stmt := range fn.Body.List {
		reads, writes := derivedFieldsIn(stmt)
		for _, r := range reads {
			// A field the same statement also assigns is the append pattern:
			// d.Xs = append(d.Xs, ...). That is order-safe.
			if assigned[r] || writes[r] {
				continue
			}
			bad = append(bad, violation{r, fset.Position(stmt.Pos())})
		}
		for w := range writes {
			assigned[w] = true
		}
	}

	sort.Slice(bad, func(i, j int) bool { return bad[i].pos.Line < bad[j].pos.Line })
	for _, v := range bad {
		t.Errorf("%s: d.%s is read before it is assigned.\n"+
			"    A Derived field computed from another Derived field depends on the order of\n"+
			"    statements in deriveDocs, which nothing checks and which has silently rendered\n"+
			"    zero into published documents twice. Compute it from `sum` instead, or move the\n"+
			"    assignment above this statement.", v.pos, v.field)
	}
}

// findFunc returns a top-level function by name.
func findFunc(file *ast.File, name string) *ast.FuncDecl {
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name && fn.Recv == nil {
			return fn
		}
	}
	return nil
}

// derivedFieldsIn reports which `d.Field` selectors a statement reads and
// which it assigns.
func derivedFieldsIn(stmt ast.Stmt) (reads []string, writes map[string]bool) {
	writes = map[string]bool{}

	// Assignment targets are writes; everything else in the statement is a
	// read, including the right-hand side of an assignment to the same field.
	// An accumulator is safe by construction: it starts at the zero value and
	// every touch is a read-modify-write of itself, so no ordering exists to
	// get wrong. d.N++ and d.N += x are both that shape.
	if inc, ok := stmt.(*ast.IncDecStmt); ok {
		if f, ok := derivedSelector(inc.X); ok {
			writes[f] = true
		}
		return nil, writes
	}
	if as, ok := stmt.(*ast.AssignStmt); ok {
		accumulate := as.Tok != token.ASSIGN && as.Tok != token.DEFINE
		for _, lhs := range as.Lhs {
			if f, ok := derivedSelector(lhs); ok {
				writes[f] = true
			}
		}
		if accumulate {
			// Only the right-hand side can reference something assigned later.
			for _, rhs := range as.Rhs {
				ast.Inspect(rhs, func(n ast.Node) bool {
					if f, ok := derivedSelector(n); ok && !writes[f] {
						reads = append(reads, f)
					}
					return true
				})
			}
			return reads, writes
		}
		for _, rhs := range as.Rhs {
			ast.Inspect(rhs, func(n ast.Node) bool {
				if f, ok := derivedSelector(n); ok {
					reads = append(reads, f)
				}
				return true
			})
		}
		return reads, writes
	}

	// Any other statement shape: nested assignments count as writes, and
	// everything read anywhere inside counts as a read.
	ast.Inspect(stmt, func(n ast.Node) bool {
		if inc, ok := n.(*ast.IncDecStmt); ok {
			if f, ok := derivedSelector(inc.X); ok {
				writes[f] = true
			}
			return false
		}
		if as, ok := n.(*ast.AssignStmt); ok {
			acc := as.Tok != token.ASSIGN && as.Tok != token.DEFINE
			for _, lhs := range as.Lhs {
				if f, ok := derivedSelector(lhs); ok {
					writes[f] = true
				}
			}
			for _, rhs := range as.Rhs {
				ast.Inspect(rhs, func(m ast.Node) bool {
					if f, ok := derivedSelector(m); ok && !(acc && writes[f]) {
						reads = append(reads, f)
					}
					return true
				})
			}
			return false
		}
		if f, ok := derivedSelector(n); ok {
			reads = append(reads, f)
		}
		return true
	})
	return reads, writes
}

// derivedSelector matches `d.Field`, the receiver deriveDocs fills.
func derivedSelector(n ast.Node) (string, bool) {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "d" {
		return "", false
	}
	return sel.Sel.Name, true
}
