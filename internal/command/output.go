package command

import (
	"bytes"
	"runtime"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func DecodeOutput(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if utf8.Valid(raw) {
		return string(raw)
	}
	if runtime.GOOS == "windows" {
		if decoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), raw); err == nil && utf8.Valid(decoded) {
			return string(decoded)
		}
	}
	return string(bytes.ToValidUTF8(raw, []byte("�")))
}
