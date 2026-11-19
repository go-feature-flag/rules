package parser

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

var inBenchSizes = []int{8, 16, 32, 64, 1000, 10000, 300000}

func inBenchList(kind string, n int) string {
	parts := make([]string, n)
	for i := range parts {
		if kind == "str" {
			parts[i] = `"user-` + strconv.Itoa(i) + `"`
		} else {
			parts[i] = strconv.Itoa(i)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// BenchmarkIn measures one Process call on a compiled evaluator for IN and
// NOT IN, with the looked-up value present (hit) and absent (miss). The hit
// is the last element, the worst case for a linear scan.
func BenchmarkIn(b *testing.B) {
	for _, kind := range []string{"str", "int"} {
		for _, n := range inBenchSizes {
			list := inBenchList(kind, n)
			var hit, miss interface{}
			if kind == "str" {
				hit, miss = "user-"+strconv.Itoa(n-1), "nobody"
			} else {
				hit, miss = n-1, -1
			}
			for _, op := range []string{"in", "not in"} {
				rule := "x " + op + " " + list
				if op == "not in" {
					rule = "not (x in " + list + ")"
				}
				ev := newBenchEvaluator(b, rule)
				for _, c := range []struct {
					name string
					v    interface{}
				}{{"hit", hit}, {"miss", miss}} {
					input := map[string]interface{}{"x": c.v}
					ev.Process(input) // compile
					name := fmt.Sprintf("%s/%s/%s/n=%d", kind, strings.ReplaceAll(op, " ", ""), c.name, n)
					b.Run(name, func(b *testing.B) {
						b.ReportAllocs()
						for i := 0; i < b.N; i++ {
							ev.Process(input)
						}
					})
				}
			}
		}
	}
}

// BenchmarkInCompile measures NewEvaluator plus the first Process call, which
// is when the rule is compiled (and the hash set built, if any).
func BenchmarkInCompile(b *testing.B) {
	for _, kind := range []string{"str", "int"} {
		for _, n := range inBenchSizes {
			rule := "x in " + inBenchList(kind, n)
			input := map[string]interface{}{"x": -1}
			if kind == "str" {
				input["x"] = "nobody"
			}
			b.Run(fmt.Sprintf("%s/n=%d", kind, n), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					newBenchEvaluator(b, rule).Process(input)
				}
			})
		}
	}
}
