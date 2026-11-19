package parser

import (
	"runtime"
	"testing"
)

func retainedHeap(t *testing.T, rule string) (float64, *Evaluator) {
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	ev, err := NewEvaluator(rule)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ev.Process(map[string]interface{}{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	runtime.ReadMemStats(&m1)
	return float64(int64(m1.HeapAlloc)-int64(m0.HeapAlloc)) / 1024, ev
}

// TestInSetHeap logs the heap a compiled list keeps after NewEvaluator, one
// Process call and three GCs (run with -v). The figure includes the rule text
// if the evaluator retains it.
func TestInSetHeap(t *testing.T) {
	for _, c := range []struct{ name, rule string }{
		{"str1k", "s in " + seqStrs(1000)},
		{"int1k", "n in " + seqInts(1000)},
		{"str300k", "s in " + seqStrs(300000)},
		{"int300k", "n in " + seqInts(300000)},
	} {
		kib, ev := retainedHeap(t, c.rule)
		t.Logf("%s retained heap %.0f KiB (rule text %.0f KiB)", c.name, kib, float64(len(c.rule))/1024)
		runtime.KeepAlive(ev)
	}
}
