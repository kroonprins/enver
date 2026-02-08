package transformations

import (
	"encoding/base64"
)

type Base64Decode struct{}

func (t *Base64Decode) Transform(input string) string {
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		// Return original input if decoding fails
		return input
	}
	return string(decoded)
}
