// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

package pdf

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
)

func floatToDecimal(v float32) string { return string(appendFloatToDecimal(nil, v)) }

func TestFloatToDecimalKnown(t *testing.T) {
	cases := []struct {
		want string
		in   float32
	}{
		{in: 0, want: "0"},
		{in: -0, want: "0"},
		{in: 1, want: "1"},
		{in: -1, want: "-1"},
		{in: 2, want: "2"},
		{in: 10, want: "10"},
		{in: 100, want: "100"},
		{in: 1000, want: "1000"},
		{in: 255, want: "255"},
		{in: 0.5, want: ".5"},
		{in: -0.5, want: "-.5"},
		{in: 0.25, want: ".25"},
		{in: 0.75, want: ".75"},
		{in: 0.125, want: ".125"},
		{in: 1.5, want: "1.5"},
		{in: 2.5, want: "2.5"},
		{in: -2.5, want: "-2.5"},
		{in: 3, want: "3"},
		{in: 72, want: "72"},
		{in: 612, want: "612"},
		{in: 792, want: "792"},
		{in: float32(math.NaN()), want: "0"},
		// ±Inf serializes as the nearest finite float, ±FLT_MAX.
		{in: float32(math.Inf(1)), want: "340282350000000000000000000000000000000"},
		{in: float32(math.Inf(-1)), want: "-340282350000000000000000000000000000000"},
	}
	for _, c := range cases {
		if got := floatToDecimal(c.in); got != c.want {
			t.Errorf("appendFloatToDecimal(%g) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFloatToDecimalRoundTrip verifies the documented output grammar /[-]?([0-9]*.)?[0-9]+/ for every float (no
// exponent, at most one sign and one dot, and always at least one digit), and the round-trip guarantee: parsing the
// output as a float recovers the exact original value for every finite float.
func TestFloatToDecimalRoundTrip(t *testing.T) {
	checkSyntax := func(s string) {
		if s == "" {
			t.Fatal("empty output")
		}
		dots, digits := 0, 0
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch {
			case c == '-':
				if i != 0 {
					t.Errorf("%q: '-' not at start", s)
				}
			case c == '.':
				dots++
			case c >= '0' && c <= '9':
				digits++
			default:
				t.Errorf("%q: unexpected character %q", s, c)
			}
		}
		if dots > 1 {
			t.Errorf("%q: more than one '.'", s)
		}
		if digits == 0 {
			t.Errorf("%q: no digits", s)
		}
	}
	verify := func(v float32) {
		s := floatToDecimal(v)
		checkSyntax(s)
		if f64 := float64(v); math.IsNaN(f64) || math.IsInf(f64, 0) {
			return
		}
		parsed, err := strconv.ParseFloat(s, 32)
		if err != nil {
			t.Fatalf("ParseFloat(%q) from %g: %v", s, v, err)
		}
		if float32(parsed) != v {
			t.Errorf("round trip %g -> %q -> %g", v, s, float32(parsed))
		}
	}
	specials := []float32{
		0, 1, -1, math.MaxFloat32, -math.MaxFloat32, math.SmallestNonzeroFloat32,
		-math.SmallestNonzeroFloat32, 0.1, 0.2, 0.3, 1.0 / 3.0, math.Pi, math.E,
		1e-30, 1e30, 16777215, 16777216, 167772159, 167772160,
	}
	for _, v := range specials {
		verify(v)
	}
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 500000; i++ {
		verify(math.Float32frombits(rng.Uint32()))
	}
}
