package parser

import (
	"hash/maphash"
	"math"
	"math/bits"
	"strings"

	"github.com/antlr4-go/antlr/v4"
)

// setSeed seeds the string hash. maphash is a fast, well mixed hash that is
// randomly seeded per process, so lookups cannot be steered into long probe
// chains by crafted keys. (maphash.String needs Go 1.19+; go.mod is 1.21.)
var setSeed = maphash.MakeSeed()

// Both sets use open addressing with linear probing over a power of two
// table holding uint32 element indexes (index+1, so 0 means empty). The table
// is sized to between 1.5 and 3 slots per element (load factor 1/3 to 2/3),
// which keeps probe chains short while costing only 4-8 bytes per element.
//
// uint32 offsets and indexes are safe because the constructors refuse to
// build a set when the element count or the total string bytes would not fit
// in a uint32; the caller then keeps the plain linear-scan list.

// strSet is a long string list stored in one allocation, plus an open
// addressing hash index over it. Compared with a []string and a
// map[string]struct{} it has no per-string allocation or header, and no map
// buckets: about 18 bytes per element for 8 byte strings instead of 75.
type strSet struct {
	data string   // all elements, concatenated
	ends []uint32 // len n+1: element i is data[ends[i]:ends[i+1]], ends[0] = 0
	tab  []uint32 // open addressing slots: element index + 1, 0 = empty
	mask uint64
}

func (s *strSet) at(i uint32) string { return s.data[s.ends[i]:s.ends[i+1]] }

func (s *strSet) has(v string) bool {
	for i := maphash.String(setSeed, v) & s.mask; ; i = (i + 1) & s.mask {
		e := s.tab[i]
		if e == 0 {
			return false
		}
		if s.at(e-1) == v {
			return true
		}
	}
}

// slice expands the list into a new []string (strings share data).
func (s *strSet) slice() []string {
	out := make([]string, len(s.ends)-1)
	for i := range out {
		out[i] = s.at(uint32(i))
	}
	return out
}

func tableSize(n int) int {
	size := 1 << bits.Len(uint(n*3/2))
	return max(size, 8)
}

// newStrSet packs the STRING tokens (quotes stripped like getString), or
// returns nil when the list is too large for 32 bit offsets.
func newStrSet(tokens []antlr.Token, src *sourceText) *strSet {
	if len(tokens) >= math.MaxUint32 {
		return nil
	}
	ends := make([]uint32, 1, len(tokens)+1)
	var b strings.Builder
	if text, ok := src.ascii(tokens[0]); ok {
		// ASCII input: rune indexes are byte indexes, so slice the source
		// text and copy once instead of one GetText allocation per element.
		total := 0
		for _, t := range tokens {
			total += max(t.GetStop()-t.GetStart()-1, 0)
		}
		if total >= math.MaxUint32 {
			return nil
		}
		b.Grow(total)
		for _, t := range tokens {
			b.WriteString(getString(text[t.GetStart() : t.GetStop()+1]))
			ends = append(ends, uint32(b.Len()))
		}
	} else {
		strs := make([]string, len(tokens))
		total := 0
		for i, t := range tokens {
			strs[i] = getString(t.GetText())
			total += len(strs[i])
		}
		if total >= math.MaxUint32 {
			return nil
		}
		b.Grow(total)
		for _, v := range strs {
			b.WriteString(v)
			ends = append(ends, uint32(b.Len()))
		}
	}
	s := &strSet{data: b.String(), ends: ends}
	size := tableSize(len(tokens))
	s.tab = make([]uint32, size)
	s.mask = uint64(size - 1)
	for i := 0; i < len(tokens); i++ {
		j := maphash.String(setSeed, s.at(uint32(i))) & s.mask
		for s.tab[j] != 0 {
			j = (j + 1) & s.mask
		}
		s.tab[j] = uint32(i + 1)
	}
	return s
}

// sourceText gives the rule text the lexer read, once per compile.
type sourceText struct {
	done    bool
	text    string
	isASCII bool
}

// ascii returns the whole rule text if every character is one byte, so token
// rune indexes can slice it.
func (s *sourceText) ascii(t antlr.Token) (string, bool) {
	if !s.done {
		s.done = true
		if cs := t.GetInputStream(); cs != nil && cs.Size() > 0 {
			s.text = cs.GetText(0, cs.Size()-1)
			s.isASCII = len(s.text) == cs.Size()
		}
	}
	return s.text, s.isASCII
}

// intSet is a hash index over a long []int, which the node keeps anyway.
// Slots come from Fibonacci hashing (multiply by 2^64/phi, keep the top bits).
// Callers must ensure len(ints) < math.MaxUint32.
type intSet struct {
	ints  []int
	tab   []uint32 // element index + 1, 0 = empty
	shift uint
}

func (s *intSet) slot(v int) uint64 {
	return (uint64(v) * 0x9E3779B97F4A7C15) >> s.shift
}

func (s *intSet) has(v int) bool {
	mask := uint64(len(s.tab) - 1)
	for i := s.slot(v); ; i = (i + 1) & mask {
		e := s.tab[i]
		if e == 0 {
			return false
		}
		if s.ints[e-1] == v {
			return true
		}
	}
}

func newIntSet(ints []int) *intSet {
	size := tableSize(len(ints))
	s := &intSet{ints: ints, tab: make([]uint32, size), shift: uint(64 - bits.Len(uint(size-1)))}
	mask := uint64(size - 1)
	for i, v := range ints {
		j := s.slot(v)
		for s.tab[j] != 0 {
			j = (j + 1) & mask
		}
		s.tab[j] = uint32(i + 1)
	}
	return s
}
