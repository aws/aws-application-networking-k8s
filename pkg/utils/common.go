package utils

// TODO: should be check by API call (Mingxi)
func ArntoId(arn string) string {
	if len(arn) == 0 {
		return ""
	}
	return arn[len(arn)-22:]
}
