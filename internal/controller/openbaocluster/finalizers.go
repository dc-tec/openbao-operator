package openbaocluster

func containsFinalizer(finalizers []string, value string) bool {
	for _, f := range finalizers {
		if f == value {
			return true
		}
	}
	return false
}

func removeFinalizer(finalizers []string, value string) []string {
	result := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f == value {
			continue
		}
		result = append(result, f)
	}
	return result
}
