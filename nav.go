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

// Package nav contains validation and request functions for the Hungarian Tax Bureau, NAV.
package nav

import "unicode/utf8"

// IsValid returns true iff the tax number has a valid checksum digit.
func IsValid(adoszam string) bool {
	if len(adoszam) < 8 {
		return false
	}
	return byte(Checksum(adoszam[:7])) == adoszam[7]
}

// Checksum returns the last (8.), checksum rune for the first 7 digit.
//
// http://muzso.hu/2011/10/26/adoszam-ellenorzo-osszeg-generator
func Checksum(adoszam string) rune {
	var sum rune
	for i, r := range adoszam {
		if i >= 7 || r == utf8.RuneError || r > utf8.RuneSelf || !('0' <= r && r <= '9') {
			break
		}
		var mul rune
		switch (i + 1) % 4 {
		case 1:
			mul = 9
		case 2:
			mul = 7
		case 3:
			mul = 3
		default:
			mul = 1
		}
		sum = (sum + mul*rune(r-'0')) % 10
	}
	if sum == 0 {
		return '0'
	}
	return rune('0' + (10 - sum))
}
