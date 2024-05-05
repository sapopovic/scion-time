package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	var fn string
	flag.StringVar(&fn, "f", "", "Mbg data file to read")
	flag.Parse()
	f, err := os.Open(fn)
	if err != nil {
		log.Fatalf("failed to open file: %s", err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		l := s.Text()
		ts := strings.Fields(l) // Tokenize the line
		if len(ts) >= 6 && ts[0] == "GNS181PEX:" {
			t := ts[1] + "T" + ts[2]
			off := ts[5]
			if len(off) != 0 && off[len(off)-1] == ',' {
				off = off[:len(off)-1]
			}
			fmt.Println(t + "," + off)
		}
	}
	if err := s.Err(); err != nil {
		log.Fatalf("error during scan: %s", err)
	}
}
