package admiral

import "strings"

func ErrorEqualOrSimilar(err1, err2 error) bool {
	if err1 != nil && err2 == nil {
		return false
	}
	if err1 != nil && err2 != nil {
		if !(err1.Error() == err2.Error() ||
			strings.Contains(err1.Error(), err2.Error())) {
			return false
		}
	}
	if err1 == nil && err2 != nil {
		return false
	}
	return true
}
