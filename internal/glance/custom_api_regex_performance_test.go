package glance

import (
	"fmt"
	"testing"
)

func BenchmarkCustomAPIRegexPatternChurn(b *testing.B) {
	findMatch := customAPITemplateFuncs["findMatch"].(func(string, string) string)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pattern := fmt.Sprintf("^glance-%d$", i)
		findMatch(pattern, fmt.Sprintf("glance-%d", i))
	}
}

func BenchmarkCustomAPIRegexCachedPattern(b *testing.B) {
	findMatch := customAPITemplateFuncs["findMatch"].(func(string, string) string)

	const pattern = "^glance-[0-9]+$"
	const value = "glance-12345"

	findMatch(pattern, value)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		findMatch(pattern, value)
	}
}
