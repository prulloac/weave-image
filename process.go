package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type imageDimensions struct {
	Width  int
	Height int
}

func lookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func runProcess(ctx context.Context, binary string, args []string, input []byte) ([]byte, string, error) {
	command := exec.CommandContext(ctx, binary, args...)
	command.WaitDelay = 100 * time.Millisecond
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	stdout := &boundedBuffer{limit: maxOutput}
	stderr := &boundedBuffer{limit: 4096}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, stderr.String(), fmt.Errorf("timed out")
	}
	if err != nil {
		return nil, stderr.String(), err
	}
	return stdout.Bytes(), stderr.String(), nil
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > b.limit {
		b.overflow = true
		return len(data), nil
	}
	return b.buffer.Write(data)
}

func (b *boundedBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *boundedBuffer) String() string { return b.buffer.String() }
