package parser

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"testing"
)

func countSets(n *node) (c int) {
	if n == nil {
		return 0
	}
	if n.cmp != nil && (n.cmp.strSet != nil || n.cmp.intSet != nil) {
		c++
	}
	return c + countSets(n.left) + countSets(n.right)
}

// seqStrs returns the list literal ["u0", ..., "u<n-1>"].
func seqStrs(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("%q", fmt.Sprintf("u%d", i))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// seqInts returns the list literal [0, ..., n-1].
func seqInts(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprint(i)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func randStrList(r *rand.Rand, n int) string {
	pool := []string{"a", "A", "u1", "U1", "", "ß", "é", "é", "1.2.3", "5", "true", "null", "x y", `q\"q`, "日本", "İ", "i"}
	parts := make([]string, n)
	for i := range parts {
		s := fmt.Sprintf("u%d", r.Intn(n*2+1))
		if r.Intn(3) == 0 {
			s = pool[r.Intn(len(pool))]
		}
		parts[i] = `"` + s + `"` // pool entries containing \" are already escaped
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// randIntList builds an int list; with bad set, one element overflows int so
// the compiled list is partial and carries a parse error (no set is used).
func randIntList(r *rand.Rand, n int, bad bool) string {
	parts := make([]string, n)
	for i := range parts {
		switch r.Intn(6) {
		case 0:
			parts[i] = "0"
		case 1:
			parts[i] = fmt.Sprint(r.Intn(10)) // the grammar has no negative list ints
		default:
			parts[i] = fmt.Sprint(r.Intn(n * 2))
		}
	}
	if bad && n > 2 {
		parts[r.Intn(n)] = "99999999999999999999"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

type kv = map[string]interface{}

// TestInSet checks IN / NOT IN results on lists above the set threshold
// against hand-written expectations and against the original visitor, which
// never uses the set.
func TestInSet(t *testing.T) {
	t.Parallel()
	strs, ints := seqStrs(100), seqInts(100)
	tests := []struct {
		name    string
		rule    string
		input   kv
		want    bool
		wantSet bool // a hash set is expected to be compiled
	}{
		{"string hit", "s in " + strs, kv{"s": "u50"}, true, true},
		{"string first", "s in " + strs, kv{"s": "u0"}, true, true},
		{"string last", "s in " + strs, kv{"s": "u99"}, true, true},
		{"string miss", "s in " + strs, kv{"s": "u100"}, false, true},
		{"string is case sensitive", "s in " + strs, kv{"s": "U50"}, false, true},
		{"string empty", "s in " + strs, kv{"s": ""}, false, true},
		{"string upper-case operator", "s IN " + strs, kv{"s": "u7"}, true, true},
		{"string stringer", "s in " + strs, kv{"s": testStringer{"u7"}}, true, true},
		{"string wrong type", "s in " + strs, kv{"s": 7}, false, true},
		{"string nil value", "s in " + strs, kv{"s": nil}, false, true},
		{"string missing attribute", "s in " + strs, kv{"other": "u1"}, false, true},
		{"string nil input", "s in " + strs, nil, false, true},
		{"string nested hit", "a.b in " + strs, kv{"a": kv{"b": "u3"}}, true, true},
		{"string nested miss", "a.b in " + strs, kv{"a": kv{"b": "zz"}}, false, true},
		{"string nested not object", "a.b in " + strs, kv{"a": "u3"}, false, true},
		{"string not in hit", "not (s in " + strs + ")", kv{"s": "u50"}, false, true},
		{"string not in miss", "not (s in " + strs + ")", kv{"s": "zz"}, true, true},
		{"string not in missing", "not (s in " + strs + ")", kv{}, true, true},
		{"string in or other", "s in " + strs + " or n eq 1", kv{"s": "zz", "n": 1}, true, true},
		{"string in and other", "s in " + strs + " and n eq 1", kv{"s": "u1", "n": 2}, false, true},
		{"int hit", "n in " + ints, kv{"n": 50}, true, true},
		{"int first", "n in " + ints, kv{"n": 0}, true, true},
		{"int last", "n in " + ints, kv{"n": 99}, true, true},
		{"int miss", "n in " + ints, kv{"n": 100}, false, true},
		{"int negative", "n in " + ints, kv{"n": -1}, false, true},
		{"int float value", "n in " + ints, kv{"n": 5.0}, false, true},
		{"int int64 value", "n in " + ints, kv{"n": int64(5)}, false, true},
		{"int string value", "n in " + ints, kv{"n": "5"}, false, true},
		{"int bool value", "n in " + ints, kv{"n": true}, false, true},
		{"int nil value", "n in " + ints, kv{"n": nil}, false, true},
		{"int missing attribute", "n in " + ints, kv{}, false, true},
		{"int nested hit", "a.n in " + ints, kv{"a": kv{"n": 9}}, true, true},
		{"int not in hit", "not (n in " + ints + ")", kv{"n": 9}, false, true},
		{"int not in miss", "not (n in " + ints + ")", kv{"n": 900}, true, true},
		{"int duplicates", "n in [" + strings.Repeat("3, ", 99) + "4]", kv{"n": 4}, true, true},
		{"float list never uses a set", "n in [" + strings.Repeat("1.0, ", 99) + "2.0]", kv{"n": 2.0}, true, false},
		{"short string list", "s in " + seqStrs(5), kv{"s": "u4"}, true, false},
		{"short int list", "n in " + seqInts(5), kv{"n": 4}, true, false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev, err := NewEvaluator(tc.rule)
			if err != nil {
				t.Fatal(err)
			}
			// the visitor must run first: it reads the parse tree that Process
			// compiles away. The second pass reuses the compiled rule.
			wantRet, wantErr, wantDebug := visitorProcess(ev.tree, tc.input)
			for pass := 0; pass < 2; pass++ {
				got, err := ev.Process(tc.input)
				if err != nil && tc.want {
					t.Fatalf("pass %d: unexpected error %v", pass, err)
				}
				if got != tc.want {
					t.Fatalf("pass %d: got %v, want %v (err %v)", pass, got, tc.want, err)
				}
				if got != wantRet || errString(err) != errString(wantErr) || errString(ev.LastDebugErr()) != errString(wantDebug) {
					t.Fatalf("pass %d: got %v/%q/%q, visitor %v/%q/%q", pass,
						got, errString(err), errString(ev.LastDebugErr()), wantRet, errString(wantErr), errString(wantDebug))
				}
			}
			if hasSet := countSets(ev.root) > 0; hasSet != tc.wantSet {
				t.Fatalf("hash set compiled = %v, want %v", hasSet, tc.wantSet)
			}
		})
	}
}

// TestInSetThreshold checks the switch from linear scan to hash set at
// inSetMinLen, and that both sides of the boundary give the same answers.
func TestInSetThreshold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		n       int
		wantSet bool
	}{
		{"one", 1, false},
		{"below", inSetMinLen - 1, false},
		{"at", inSetMinLen, true},
		{"above", inSetMinLen + 1, true},
	}
	for _, kind := range []string{"string", "int"} {
		for _, tc := range tests {
			tc := tc
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				var rule string
				var first, last, miss interface{}
				if kind == "string" {
					rule, first, last, miss = "x in "+seqStrs(tc.n), "u0", fmt.Sprintf("u%d", tc.n-1), fmt.Sprintf("u%d", tc.n)
				} else {
					rule, first, last, miss = "x in "+seqInts(tc.n), 0, tc.n-1, tc.n
				}
				ev, err := NewEvaluator(rule)
				if err != nil {
					t.Fatal(err)
				}
				for _, c := range []struct {
					v    interface{}
					want bool
				}{{first, true}, {last, true}, {miss, false}} {
					got, err := ev.Process(kv{"x": c.v})
					if err != nil || got != c.want {
						t.Fatalf("x=%v: got %v, %v; want %v", c.v, got, err, c.want)
					}
				}
				if hasSet := countSets(ev.root) > 0; hasSet != tc.wantSet {
					t.Fatalf("hash set compiled = %v, want %v", hasSet, tc.wantSet)
				}
			})
		}
	}
}

// TestInSetDifferential compares compiled evaluation with the original
// visitor over seeded random lists and a spread of awkward inputs: results,
// errors and debug errors must be identical.
func TestInSetDifferential(t *testing.T) {
	t.Parallel()
	vals := []interface{}{"u1", "U1", "u3", "a", "A", "", "é", "é", "ß", "SS", "5", "1.2.3", "null", "i", "İ", "日本",
		0, 1, 3, -1, 5, 15, int64(3), int32(3), uint(3), 3.0, 2.0, 0.0, math.Copysign(0, -1), math.NaN(), math.Inf(1), float32(3),
		true, false, nil, []string{"u1"}, []int{3}, kv{"a": 1}, testStringer{"u1"}, testStringer{"U1"}, fmt.Stringer(nil), "\xff"}
	inputs := []kv{nil, {}}
	for _, v := range vals {
		inputs = append(inputs, kv{"s": v, "n": v, "u": kv{"s": v, "n": v}})
	}
	inputs = append(inputs, kv{"n": 3, "s": "u1"}, kv{"n": "x", "s": 3}, kv{"other": 1})

	for _, size := range []int{1, 7, inSetMinLen - 1, inSetMinLen, inSetMinLen + 1, 200} {
		size := size
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			t.Parallel()
			r := rand.New(rand.NewSource(int64(7 + size)))
			var rules []string
			for k := 0; k < 2; k++ {
				sl, il, ilBad := randStrList(r, size), randIntList(r, size, false), randIntList(r, size, true)
				rules = append(rules,
					"s in "+sl, "s IN "+sl, "u.s in "+sl, "not (s in "+sl+")", "NOT (n in "+il+")",
					"n in "+il, "u.n in "+il, "n in "+ilBad, "n in "+ilBad+" or s in "+sl,
					"s in "+sl+" and n in "+il, "(s in "+sl+") or (n in "+ilBad+")",
					"s in "+sl+" or s in "+randStrList(r, size), "missing in "+sl, "missing in "+il,
					"s in "+sl+" or n in "+il+" or s eq \"u1\"",
					"n in "+il+" or n in "+randIntList(r, size, false),
					"s in "+sl+" or s pr and not (n in "+il+")",
				)
			}
			rules = append(rules,
				`n in [1.0,2.0,3.0,4.0,5.0,6.0,7.0,8.0,9.0]`, `n in [1,2,3,4,5,6,7,8,9,1.5]`,
				`s in ["a","b","c","d","e","f","g","h",1]`, `n in [0,0,0,0,0,0,0,0,0]`, `n in [-0,1,2,3,4,5,6,7,8]`)

			sets := 0
			for _, rule := range rules {
				repeated, err := NewEvaluator(rule)
				if err != nil {
					continue // invalid mixed lists are rejected at parse time
				}
				tree := repeated.tree
				for i, in := range inputs {
					wantRet, wantErr, wantDebug := visitorProcess(tree, in)
					fresh := &Evaluator{rule: rule, tree: tree}
					for name, ev := range map[string]*Evaluator{"fresh": fresh, "repeated": repeated} {
						gotRet, gotErr := ev.Process(in)
						if gotRet != wantRet || errString(gotErr) != errString(wantErr) || errString(ev.LastDebugErr()) != errString(wantDebug) {
							t.Errorf("%.80q input %d %v (%s): got %v/%q/%q want %v/%q/%q", rule, i, in, name,
								gotRet, errString(gotErr), errString(ev.LastDebugErr()), wantRet, errString(wantErr), errString(wantDebug))
						}
					}
				}
				sets += countSets(repeated.root)
			}
			if size >= inSetMinLen && sets == 0 {
				t.Fatal("no hash set was built: test is vacuous")
			}
			if size < inSetMinLen && sets != 0 {
				t.Fatalf("%d hash sets built below the threshold", sets)
			}
		})
	}
}

// TestInSetConcurrent evaluates one compiled evaluator from many goroutines;
// run with -race to check that the set is read-only after compile.
func TestInSetConcurrent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rule string
		val  func(i int) interface{}
		want func(i int) bool
	}{
		{"string", "s in " + seqStrs(300), func(i int) interface{} { return fmt.Sprintf("u%d", i%600) }, func(i int) bool { return i%600 < 300 }},
		{"int", "s in " + seqInts(300), func(i int) interface{} { return i % 600 }, func(i int) bool { return i%600 < 300 }},
		{"string not in", "not (s in " + seqStrs(300) + ")", func(i int) interface{} { return fmt.Sprintf("u%d", i%600) }, func(i int) bool { return i%600 >= 300 }},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev, err := NewEvaluator(tc.rule)
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			for g := 0; g < 16; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < 1000; i++ {
						if got, _ := ev.Process(kv{"s": tc.val(i)}); got != tc.want(i) {
							t.Errorf("i=%d: got %v, want %v", i, got, tc.want(i))
							return
						}
					}
				}()
			}
			wg.Wait()
		})
	}
}
