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
		fmt.Println(ts)
	}
	if err := s.Err(); err != nil {
		log.Fatalf("error during scan: %s", err)
	}
}
