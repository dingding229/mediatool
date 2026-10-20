package cliprocess

import (
	"flag"
	"os"
	"reflect"
	"testing"
)

func TestProcessAdapterRestoresFlagsAndPreservesLeafArguments(t *testing.T) {
	before := append([]string{}, os.Args...)
	flags := flag.CommandLine
	Invoke("fixture", []string{"-value", "owned"}, func() {
		v := flag.String("value", "", "owned fixture")
		flag.Parse()
		if *v != "owned" || flag.CommandLine.Name() != "fixture" {
			t.Fatal(*v)
		}
	})
	if !reflect.DeepEqual(before, os.Args) || flag.CommandLine != flags {
		t.Fatal("process state not restored")
	}
}
