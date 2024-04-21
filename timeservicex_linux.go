//go:build linux

package main

import "example.com/scion-time/core/sync/flash/adjustments"

func testAdjustments() {
	var a adjustments.Adjustment = &adjustments.Adjtimex{}
	_ = a
}
