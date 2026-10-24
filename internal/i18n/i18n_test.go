package i18n

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestCatalogCoverageAndFormatParameters(t *testing.T) {
	raw, err := os.ReadFile("en.json")
	if err != nil {
		t.Fatal(err)
	}
	var english map[string]string
	if err := json.Unmarshal(raw, &english); err != nil {
		t.Fatal(err)
	}
	if len(english) != len(arabic) {
		t.Fatal("catalogs have different keys")
	}
	formats := regexp.MustCompile(`%(?:\[[0-9]+\])?[+# .0-9-]*[a-zA-Z%]`)
	for key := range english {
		translation, ok := arabic[key]
		if !ok || translation == "" {
			t.Errorf("missing Arabic: %q", key)
		}
		if !reflect.DeepEqual(formats.FindAllString(key, -1), formats.FindAllString(translation, -1)) {
			t.Errorf("format mismatch: %q -> %q", key, translation)
		}
	}
	// Parse every OS's sources, not only files selected by this test runner.
	err = filepath.Walk("../..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != ".." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		check := func(e ast.Expr) {
			if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				key, _ := strconv.Unquote(lit.Value)
				if key != "" {
					if _, ok := arabic[key]; !ok {
						t.Errorf("%s: missing catalog entry %q", path, key)
					}
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok && x.Name == "i18n" && sel.Sel.Name == "T" && len(c.Args) == 1 {
						check(c.Args[0])
					}
					if sel.Sel.Name == "control" && len(c.Args) > 1 {
						check(c.Args[1])
					}
				}
			}
			if v, ok := n.(*ast.ValueSpec); ok && len(v.Names) == 1 && v.Names[0].Name == "usage" && len(v.Values) == 1 {
				check(v.Values[0])
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestLanguagePersistenceAndBidi(t *testing.T) {
	defer Set("en")
	dir := t.TempDir()
	if err := Save(dir, "ar"); err != nil {
		t.Fatal(err)
	}
	Set("en")
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if !RTL() || T("Paused") == "Paused" {
		t.Fatal("saved Arabic not loaded")
	}
	if LTR("Wi-Fi 2") != "\u2066Wi-Fi 2\u2069" {
		t.Fatal("missing bidi isolation")
	}
	if Set("invalid") {
		t.Fatal("invalid locale accepted")
	}
	if err := Save(dir, "en"); err != nil {
		t.Fatal(err)
	}
	if T("Paused") != "Paused" || LTR("A") != "A" {
		t.Fatal("English regression")
	}
}
