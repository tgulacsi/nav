/*
Copyright 2015 Tamás Gulácsi

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nav

import "testing"

func TestChecksum(t *testing.T) {
	for i, test := range []struct {
		taxno string
		cs    rune
	}{
		{"8888888", '8'},
		{"1234567", '6'},
		{"8888889", '5'},
		{"1389545", '9'},
	} {
		got := Checksum(test.taxno)
		if got != test.cs {
			t.Errorf("%d. %q: awaited %c, got %c.", i, test.taxno, test.cs, got)
		}
	}
}

func TestIsValid(t *testing.T) {
	for i, test := range []struct {
		taxno string
		valid bool
	}{
		{"88888888", true},
		{"12345676", true},
		{"13895459", true},

		{"12345678", false},
		{"", false},
		{"123456789", false},

		{"888888889", true},
		{"888888899", false},
	} {
		got := IsValid(test.taxno)
		if got != test.valid {
			t.Errorf("%d. %q: awaited %t, got %t.", i, test.taxno, test.valid, got)
		}
	}
}
