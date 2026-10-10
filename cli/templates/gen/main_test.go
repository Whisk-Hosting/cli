package main

import (
	"reflect"
	"testing"
)

func TestParts(t *testing.T) {
	e := func(p string, n int) entry { return entry{p, string(make([]byte, n))} }
	names := func(ps [][]entry) [][]string {
		out := [][]string{}
		for _, p := range ps {
			var n []string
			for _, x := range p {
				n = append(n, x.path)
			}
			out = append(out, n)
		}
		return out
	}
	for _, c := range []struct {
		name string
		in   []entry
		want [][]string
	}{
		{"nothing", nil, [][]string{}},
		{"all fit", []entry{e("a", 3), e("b", 4)}, [][]string{{"a", "b"}}},
		{"split at the budget", []entry{e("a", 6), e("b", 4), e("c", 1)}, [][]string{{"a", "b"}, {"c"}}},
		{"a big file has a part of its own", []entry{e("a", 2), e("big", 30), e("c", 2)}, [][]string{{"a"}, {"big"}, {"c"}}},
		{"a big file first", []entry{e("big", 30), e("b", 2)}, [][]string{{"big"}, {"b"}}},
	} {
		if got := names(parts(c.in, 10)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
