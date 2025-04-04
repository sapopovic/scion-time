// Simple tool to parse the output of Meinberg's Service Daemon mbgsvcd and plot
// the recorded offset values over time.
//
// Example command to get input data: mbgsvcd -f -Q -s 1
//
// See also:
// - https://kb.meinbergglobal.com/kb/driver_software/command_line_tools_mbgtools#mbgsvcd
// - https://git.meinbergglobal.com/drivers/mbgtools-lx.git/tree/mbgsvcd/mbgsvcd.c
// - https://www.meinbergglobal.com/english/sw/#linux
// ./offplot mbg-output-file timeservice-log
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image/color"
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

/*
type interleavedStruct struct {
	time time.Time
	b    bool
}
*/

/*
	func get_times(fn string) []interleavedStruct {
		interleaved_mode_t := make([]interleavedStruct, 0) // contains times where interleaved mode was stated
		f, err := os.Open(fn)
		if err != nil {
			log.Fatalf("failed to open file: '%s', %s", fn, err)
		}
		defer f.Close()

		s := bufio.NewScanner(f)
		for s.Scan() {
			l := s.Text()
			ts := strings.Fields(l)
			var temp_t string
			var temp_t_struct time.Time
			print("HEELLLOOOOO\n")
			if len(ts) >= 13 && (ts[12] == "interleaved=false" || ts[12] == "interleaved=true") {
				print(ts[12])
				if strings.HasPrefix(ts[5], "time=") == true {
					temp_t = ts[5][5:] // remove "time=" and only save time
					temp_t_struct, _ = time.Parse(time.RFC3339, temp_t)
					b, _ := strconv.ParseBool(ts[12][12:])
					interleaved_mode_t = append(interleaved_mode_t, interleavedStruct{temp_t_struct, b}) // true or false
					print(temp_t_struct.String())

				}
			}
		}

		return interleaved_mode_t
	}
*/
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

	// no_interleave_modes := get_times(flag.Arg(1))

	n := 0
	var t0, start_time time.Time
	var data, data_not_ok plotter.XYs
	gitRevision := ""

	s := bufio.NewScanner(f0)
	for s.Scan() {
		l := s.Text()
		ts := strings.Fields(l)

		if strings.HasPrefix(l, "GIT_REVISION") {
			parts := strings.Split(l, "=")
			if len(parts) > 1 {
				gitRevision = strings.TrimSpace(parts[1])
			}
			continue
		}

		var ok bool
		var ok2 bool
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
			st := ts[16]
			if len(st) != 0 && st[len(st)-1] == ',' {
				st = st[:len(st)-1]
			}
			ok2 = true

			if st != "0x0014" {
				ok2 = false
			} else {
				ok2 = true
			}
			/*
				fmt.Println("Length of the slice:", len(no_interleave_modes))
				for i := 0; i < len(no_interleave_modes)-1; i++ { // last element not reached through loop
					stct_prev := no_interleave_modes[i]
					stct_next := no_interleave_modes[i+1]
					fmt.Println(t.String())
					fmt.Println(stct_prev.time.String())
					fmt.Println(stct_next.time.String())
					fmt.Println(stct_prev.b)
					fmt.Println()
					if i == 0 && stct_prev.b == false && t.Compare(stct_prev.time) == -1 {
						ok2 = false
						fmt.Println("here?")
						break
					} else if (t.Compare(stct_prev.time) == 1 || t.Compare(stct_prev.time) == 0) && t.Compare(stct_next.time) == -1 && stct_prev.b == false {
						ok2 = false
						// no_interleave_modes = no_interleave_modes[i+1:] // optimization
						break
					} else if (t.Compare(stct_prev.time) == -1 || t.Compare(stct_prev.time) == 0) && t.Compare(stct_prev.time) == 0 && t.Compare(stct_next.time) == -1 {
						ok2 = false
						break
					} else {
						ok2 = true
					}
				}
				fmt.Println("Do we continue?")
			*/
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
			if ok2 { // add to data
				if n == 0 {
					t0 = t
					start_time = t
				}
				data = append(data, plotter.XY{
					X: float64(t.Unix() - t0.Unix()),
					Y: off,
				})
			} else { // add to data_not_ok
				if n == 0 {
					t0 = t
					start_time = t
				}
				data_not_ok = append(data_not_ok, plotter.XY{
					X: float64(t.Unix() - t0.Unix()),
					Y: off,
				})
			}
			n++
		}
	}
	if err := s.Err(); err != nil {
		log.Fatalf("error during scan: %s", err)
	}

	p := plot.New()
	p.X.Label.Text = "Time [s]"
	p.X.Label.Padding = vg.Points(5)
	p.Y.Label.Text = "Offset [s]"
	p.Y.Label.Padding = vg.Points(5)

	if gitRevision != "" {
		p.Title.Text = fmt.Sprintf("SCION Offset (Rev: %s)", gitRevision) + "\nMeasurement started at: " + start_time.Format(time.DateTime)
	} else {
		p.Title.Text = "Chrony Offset\n" + "Measurement started at: " + start_time.Format(time.DateTime)
	}

	p.Add(plotter.NewGrid())

	scatter, err := plotter.NewScatter(data)
	scatter2, err2 := plotter.NewScatter(data_not_ok)
	if err != nil {
		log.Fatalf("error during plot: %s", err)
	}
	if err2 != nil {
		log.Fatalf("error during plot: %s", err2)
	}

	scatter.GlyphStyle.Shape = draw.CircleGlyph{}
	scatter2.GlyphStyle.Shape = draw.CircleGlyph{}
	scatter.GlyphStyle.Color = color.RGBA{R: 0, G: 0, B: 0, A: 255}
	scatter2.GlyphStyle.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
	scatter.GlyphStyle.Radius = vg.Points(1)
	scatter2.GlyphStyle.Radius = vg.Points(1)

	p.Add(scatter)
	p.Add(scatter2)

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
