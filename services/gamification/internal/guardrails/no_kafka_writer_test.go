// v0
// internal/guardrails/no_kafka_writer_test.go
package guardrails

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoKafkaWriterUsage guards the gamification service from accidentally wiring
// Kafka producer APIs. The gamification service must only consume from Kafka, so
// any attempt to import kafka-go writer helpers or types immediately fails the
// build via this unit test.
func TestNoKafkaWriterUsage(t *testing.T) {
	serviceRoot := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(serviceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		aliases := kafkaAliases(file)
		if len(aliases) == 0 {
			return nil
		}
		if _, hasDot := aliases["."]; hasDot {
			t.Errorf("dot-import of kafka-go detected in %s", path)
			return nil
		}

		ast.Inspect(file, func(n ast.Node) bool {
			selector, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, exists := aliases[ident.Name]; !exists {
				return true
			}
			if selector.Sel != nil {
				switch selector.Sel.Name {
				case "Writer", "NewWriter":
					t.Errorf("kafka producer usage detected in %s", path)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk gamification service: %v", err)
	}
}

func kafkaAliases(file *ast.File) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, "\"")
		if path != "github.com/segmentio/kafka-go" {
			continue
		}
		if spec.Name != nil {
			name := spec.Name.Name
			if name == "." {
				// Dot-imports automatically expose writer helpers; fail fast.
				aliases["."] = struct{}{}
				continue
			}
			if name != "_" && name != "" {
				aliases[name] = struct{}{}
				continue
			}
		}
		aliases["kafka"] = struct{}{}
	}
	return aliases
}
