package runfile_test

import (
	"flag"
	"testing"
)

// concurrentStartPath is set by the -concurrent-start-path flag when the test
// binary is invoked as a subprocess for TestConcurrentStartHelper.
var concurrentStartPath string

func TestMain(m *testing.M) {
	flag.StringVar(&concurrentStartPath, "concurrent-start-path", "", "path for TestConcurrentStartHelper")
	flag.Parse()
	m.Run()
}
