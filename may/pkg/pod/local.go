package pod

// --- Option 1: Static exclusion list ---
// Well-known flavor names that run locally without remote host provisioning.

var localFlavors = map[string]struct{}{
	"localhost": {},
	"local":     {},
}

func IsLocalFlavor(flavor string) bool {
	_, ok := localFlavors[flavor]
	return ok
}
