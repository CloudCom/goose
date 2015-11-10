package main

import (
	"bytes"
	"io"
	"os"
	"sync"
)

func run(args []string, env map[string]string) (int, string, error) {
	for name, val := range env {
		defer os.Setenv(name, os.Getenv(name))
		os.Setenv(name, val)
	}

	defer func(args []string) { os.Args = args }(os.Args)
	os.Args = append([]string{"goose"}, args...)

	defer func(f *os.File) { os.Stdout = f }(os.Stdout)
	r, w, err := os.Pipe()
	if err != nil {
		return 0, "", err
	}
	defer w.Close()
	buf := bytes.NewBuffer(nil)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() { io.Copy(buf, r); wg.Done() }()
	os.Stdout = w

	status := imain()

	w.Close()
	wg.Wait()

	return status, string(buf.Bytes()), nil
}
