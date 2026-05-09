package utils

import "time"

// jakartaTZ is cached at package load to avoid repeated tzdata lookups.
var jakartaTZ *time.Location

func init() {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// fallback: fixed +07:00 if tzdata is absent (e.g. distroless without zoneinfo)
		loc = time.FixedZone("WIB", 7*3600)
	}
	jakartaTZ = loc
}

// JakartaTZ returns the *time.Location for WIB (Asia/Jakarta).
func JakartaTZ() *time.Location { return jakartaTZ }

// NowJakarta returns the current time in WIB.
func NowJakarta() time.Time { return time.Now().In(jakartaTZ) }
