package glance

import (
	"html/template"
	"strings"

	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/registry/conversion"
	"github.com/go-sprout/sprout/registry/encoding"
	"github.com/go-sprout/sprout/registry/maps"
	"github.com/go-sprout/sprout/registry/numeric"
	"github.com/go-sprout/sprout/registry/regex"
	"github.com/go-sprout/sprout/registry/semver"
	"github.com/go-sprout/sprout/registry/slices"
	"github.com/go-sprout/sprout/registry/std"
	sproutstrings "github.com/go-sprout/sprout/registry/strings"
)

var customAPISproutAllowedFunctions = map[string]struct{}{
	// Conversion.
	"toBool": {}, "toInt": {}, "toInt64": {}, "toUint": {}, "toUint64": {},
	"toFloat64": {}, "toOctal": {}, "toString": {}, "toDate": {},
	"toLocalDate": {}, "toDuration": {},

	// Strings. shuffle is intentionally excluded because it is nondeterministic.
	"nospace": {}, "trim": {}, "trimAll": {}, "trimPrefix": {}, "trimSuffix": {},
	"contains": {}, "hasPrefix": {}, "hasSuffix": {}, "toLower": {}, "toUpper": {},
	"replace": {}, "repeat": {}, "join": {}, "trunc": {}, "ellipsis": {},
	"ellipsisBoth": {}, "initials": {}, "plural": {}, "wrap": {}, "wrapWith": {},
	"quote": {}, "squote": {}, "toCamelCase": {}, "toKebabCase": {},
	"toPascalCase": {}, "toDotCase": {}, "toPathCase": {}, "toConstantCase": {},
	"toSnakeCase": {}, "toTitleCase": {}, "untitle": {}, "swapCase": {},
	"capitalize": {}, "uncapitalize": {}, "split": {}, "splitn": {}, "substr": {},
	"indent": {}, "nindent": {}, "seq": {}, "escape": {}, "unescape": {},

	// Slices.
	"list": {}, "append": {}, "prepend": {}, "concat": {}, "chunk": {}, "uniq": {},
	"compact": {}, "flatten": {}, "flattenDepth": {}, "slice": {}, "has": {},
	"without": {}, "rest": {}, "initial": {}, "first": {}, "last": {}, "reverse": {},
	"sortAlpha": {}, "splitList": {}, "strSlice": {}, "until": {}, "untilStep": {},

	// Maps.
	"dict": {}, "get": {}, "set": {}, "unset": {}, "keys": {}, "values": {},
	"pluck": {}, "pick": {}, "omit": {}, "dig": {}, "hasKey": {}, "merge": {},
	"mergeOverwrite": {},

	// Regular expressions.
	"regexFind": {}, "regexFindAll": {}, "regexMatch": {}, "regexSplit": {},
	"regexReplaceAll": {}, "regexReplaceAllLiteral": {}, "regexQuoteMeta": {},
	"regexFindGroups": {}, "regexFindAllGroups": {}, "regexFindNamed": {},
	"regexFindAllNamed": {},

	// Numeric.
	"floor": {}, "ceil": {}, "round": {}, "add": {}, "add1": {}, "sub": {},
	"mul": {}, "mulf": {}, "div": {}, "divf": {}, "mod": {}, "min": {},
	"minf": {}, "max": {}, "maxf": {},

	// Standard helpers. hello is intentionally excluded.
	"default": {}, "empty": {}, "all": {}, "any": {}, "coalesce": {},
	"ternary": {}, "cat": {},

	// Encoding.
	"base64Encode": {}, "base64Decode": {}, "base32Encode": {}, "base32Decode": {},
	"fromJSON": {}, "toJSON": {}, "toPrettyJSON": {}, "toRawJSON": {},
	"fromYAML": {}, "toYAML": {}, "toIndentYAML": {},

	// Semantic versions.
	"semver": {}, "semverCompare": {},
}

func customAPISproutTemplateFuncs() template.FuncMap {
	handler := sprout.New()

	handler.AddRegistries(
		conversion.NewRegistry(),
		sproutstrings.NewRegistry(),
		slices.NewRegistry(),
		maps.NewRegistry(),
		regex.NewRegistry(),
		numeric.NewRegistry(),
		std.NewRegistry(),
		encoding.NewRegistry(),
		semver.NewRegistry(),
	)

	sproutFuncs := handler.Build()
	funcs := make(template.FuncMap, len(sproutFuncs))

	for name, fn := range sproutFuncs {
		if _, allowed := customAPISproutAllowedFunctions[name]; !allowed {
			continue
		}

		prefixedName := "sprout" + strings.ToUpper(name[:1]) + name[1:]
		if _, exists := funcs[prefixedName]; exists {
			panic("duplicate Custom API Sprout template function: " + prefixedName)
		}

		funcs[prefixedName] = fn
	}

	return funcs
}
