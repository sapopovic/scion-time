// Driver for quick experiments

package main

import (
	"fmt"
	"time"

	"example.com/scion-time/go/core"
)

func runX() {
	clk := &core.SystemClock{}
	t0 := clk.Now()
	fmt.Println(t0)
	clk.Step(10 * time.Minute)
	t1 := clk.Now()
	fmt.Println(t1)
}