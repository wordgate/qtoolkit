package util

import (
	"github.com/mcuadros/go-version"
)

func VersionCompare(a, b string, operation string) bool {
	return version.Compare(a, b, operation)
}