package helper

import (
	"strings"
)

// Contains tells whether a contains x.
func Contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

func ToArray(s string, separator string) []string {
	if separator == "" {
		return []string{s}
	}
	s = strings.Replace(s, " ", "", -1)
	strReplace := separator + separator
	for strings.Contains(s, strReplace) {
		s = strings.Replace(s, strReplace, separator, -1)
	}
	s = RemoveIfISLast(s, separator)
	if string(s[0]) == separator {
		s = s[1:]
	}
	return strings.Split(s, separator)
}

func RemoveIfISLast(s string, substring string) string {
	if len(s)-1 == strings.LastIndex(s, substring) {
		s = s[:len(s)-1]
	}
	return s
}
