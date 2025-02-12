// Simple tool to parse the output of Meinberg's Service Daemon mbgsvcd and plot
// the recorded offset values over time.
//
// Example command to get input data: mbgsvcd -f -Q -s 1
//
// See also:
// - https://kb.meinbergglobal.com/kb/driver_software/command_line_tools_mbgtools#mbgsvcd
// - https://git.meinbergglobal.com/drivers/mbgtools-lx.git/tree/mbgsvcd/mbgsvcd.c
// - https://www.meinbergglobal.com/english/sw/#linux

package main

import (
	"bufio"
	"flag"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgpdf"
)

func main() {
	var limit float64
	flag.Float64Var(&limit, "l", 0.0, "limit")
	flag.Parse()

	fn0 := flag.Arg(0)
	f0, err := os.Open(fn0)
	if err != nil {
		log.Fatalf("failed to open file: '%s', %s", fn0, err)
	}
	defer f0.Close()

	n := 0
	var t0 time.Time
	var data plotter.XYs

	s := bufio.NewScanner(f0)
	for s.Scan() {
		l := s.Text()
		ts := strings.Fields(l)
		var ok bool
		var t time.Time
		var off float64
		if len(ts) >= 6 && ts[0] == "GNS181PEX:" {
			x := ts[1] + "T" + ts[2] + "Z"
			t, err = time.Parse(time.RFC3339, x)
			if err != nil {
				log.Fatalf("failed to parse timestamp on line: %s, %s", l, err)
			}
			y := ts[5]
			if len(y) != 0 && y[len(y)-1] == ',' {
				y = y[:len(y)-1]
			}
			off, err = strconv.ParseFloat(y, 64)
			if err != nil {
				log.Fatalf("failed to parse offset on line: %s, %s", l, err)
			}
			ok = true
		} else if len(ts) >= 10 &&
			strings.HasPrefix(ts[0], "phc2sys[") &&
			strings.HasSuffix(ts[0], "]:") {
			x := ts[0]
			x, _ = strings.CutPrefix(x, "phc2sys[")
			x, _ = strings.CutSuffix(x, "]:")
			seconds, err := strconv.ParseFloat(x, 64)
			if err != nil {
				log.Fatalf("failed to parse timestamp on line: %s, %s", l, err)
			}
			secs := int64(seconds)
			nsecs := int64((seconds - float64(secs)) * 1e9)
			t = time.Unix(secs, nsecs).UTC()
			y, err := strconv.ParseInt(ts[4], 10, 64)
			if err != nil {
				log.Fatalf("failed to parse offset on line: %s, %s", l, err)
			}
			off = float64(y) / 1e9
			ok = true
		}
		if ok {
			if n == 0 {
				t0 = t
			}
			data = append(data, plotter.XY{
				X: float64(t.Unix() - t0.Unix()),
				Y: off,
			})
		}
		n++
	}
	if err := s.Err(); err != nil {
		log.Fatalf("error during scan: %s", err)
	}

	p := plot.New()
	p.X.Label.Text = "Time [s]"
	p.X.Label.Padding = vg.Points(5)
	p.Y.Label.Text = "Offset [s]"
	p.Y.Label.Padding = vg.Points(5)

	p.Add(plotter.NewGrid())

	line, err := plotter.NewLine(data)
	if err != nil {
		log.Fatalf("error during plot: %s", err)
	}
	p.Add(line)

	if limit != 0.0 {
		p.Y.Max = math.Abs(limit)
		p.Y.Min = -math.Abs(limit)
	}

	c := vgpdf.New(8.5*vg.Inch, 3*vg.Inch)
	c.EmbedFonts(true)
	dc := draw.New(c)
	dc = draw.Crop(dc, 1*vg.Millimeter, -1*vg.Millimeter, 1*vg.Millimeter, -1*vg.Millimeter)

	p.Draw(dc)

	fext := filepath.Ext(fn0)
	fn1 := fn0[:len(fn0)-len(fext)] + ".pdf"
	f1, err := os.Create(fn1)
	if err != nil {
		log.Fatalf("failed to create file: %s, %s", fn1, err)
	}
	defer f1.Close()
	_, err = c.WriteTo(f1)
	if err != nil {
		log.Fatalf("failed to write file: %s, %s", fn1, err)
	}
}
