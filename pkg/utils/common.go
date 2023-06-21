package utils

import "strings"

// TODO: should be check by API call (Mingxi)
func ArntoId(arn string) string {
	if len(arn) == 0 {
		return ""
	}
	return arn[len(arn)-22:]
}

func Truncate(name string, length int) string {
	if len(name) > length {
		name = name[:length]
	}
	return strings.Trim(name, "-")
}
