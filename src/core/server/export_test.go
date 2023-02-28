package server

import (
	"testing"
)

var HandleRequest = handleRequest

func LogTSS(t *testing.T, prefix string) {
	t.Helper()
	t.Logf("%s:tss = %+v", prefix, tss)
	t.Logf("%s:tssQ = %+v", prefix, tssQ)
}
