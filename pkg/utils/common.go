package utils

// {"arn":"arn:aws:mercury:us-east-1:649797057964:mesh/mesh-0f94edeeb1f6d63c6","id":"mesh-0f94edeeb1f6d63c6","name":"20220120mesh"}

// TODO: should be check by API call (Mingxi)
func ArntoId(arn string) string {
	if len(arn) == 0 {
		return ""
	}
	return arn[len(arn)-22:]
}
