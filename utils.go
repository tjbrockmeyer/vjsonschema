package vjsonschema

import (
	"fmt"
	"regexp"
)

var (
	refRegex = regexp.MustCompile(`"\$ref"\s*:\s*"{([^"]*?)}"`)
)

// Replaces all $ref values that are surrounded by { and } using the provided replacement function.
func SchemaRefReplace(schema []byte, replaceFunc func(ref string) string) []byte {
	return refRegex.ReplaceAllFunc(schema, func(match []byte) []byte {
		ref := refRegex.FindSubmatch(match)[1]
		return []byte(fmt.Sprintf(`"$ref":"%s"`, replaceFunc(string(ref))))
	})
}
