package transformations

import (
	"encoding/base64"
)

type Base64Encode struct{}

func (t *Base64Encode) Transform(input string) string {
	return base64.StdEncoding.EncodeToString([]byte(input))
}
