package pod

import (
	"strings"
)

const (
	KueueFlavorLabelPrefix string = "kueue.konflux-ci.dev/requests-"
)

func HasFlavor(annotations map[string]string) bool {
	for l := range annotations {
		if strings.HasPrefix(l, KueueFlavorLabelPrefix) {
			return true
		}
	}
	return false
}

func ExtractFlavor(annotations map[string]string) (string, bool) {
	for l := range annotations {
		if flavor, found := strings.CutPrefix(l, KueueFlavorLabelPrefix); found {
			return flavor, true
		}
	}
	return "", false
}
