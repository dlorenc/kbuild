package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/dlorenc/kbuild/pkg/snapshot"
	"github.com/docker/docker/builder/dockerfile/instructions"
	"github.com/docker/docker/builder/dockerfile/parser"
)

func exit(err error) {
	fmt.Println(err)
	os.Exit(1)
}

const Dockerfile = `FROM gcr.io/google-appengine/debian9
RUN apt-get update && apt-get install -y curl
RUN echo "hey" > /etc/foo
RUN echo "baz" > /etc/foo
RUN echo "baz" > /etc/foo2

COPY . foo
`

func main() {
	stages, err := ParseDockerfile()
	if err != nil {
		exit(err)
	}

	commandsToRun := [][]string{}
	for _, s := range stages {
		for _, cmd := range s.Commands {
			switch c := cmd.(type) {
			case *instructions.RunCommand:
				newCommand := []string{}
				if c.PrependShell {
					newCommand = []string{"sh", "-c"}
					newCommand = append(newCommand, strings.Join(c.CmdLine, " "))
				} else {
					newCommand = c.CmdLine
				}
				commandsToRun = append(commandsToRun, newCommand)
			}
		}
	}

	hasher := func(p string) string {
		h := md5.New()
		fi, err := os.Stat(p)
		if err != nil {
			exit(err)
		}
		h.Write([]byte(fi.Mode().String()))
		h.Write([]byte(fi.ModTime().String()))

		if fi.Mode().IsRegular() {
			f, err := os.Open(p)
			if err != nil {
				exit(err)
			}
			defer f.Close()
			if _, err := io.Copy(h, f); err != nil {
				exit(err)
			}
		}

		return hex.EncodeToString(h.Sum(nil))
	}

	l := snapshot.NewLayeredMap(hasher)
	snapshotter := snapshot.NewSnapshotter(l, "/work-dir")

	// Take initial snapshot
	if err := snapshotter.Init(); err != nil {
		exit(err)
	}

	for _, c := range commandsToRun {
		fmt.Println("cmd: ", c[0])
		fmt.Println("args: ", c[1:])
		if err != nil {
			exit(err)
		}
		cmd := exec.Command(c[0], c[1:]...)
		combout, err := cmd.CombinedOutput()
		if err != nil {
			exit(err)
		}
		fmt.Printf("Output from %s %s\n", cmd.Path, cmd.Args)
		fmt.Print(string(combout))

		if err := snapshotter.TakeSnapshot(); err != nil {
			exit(err)
		}
	}
}

func ParseDockerfile() ([]instructions.Stage, error) {
	d := strings.NewReader(Dockerfile)
	r, err := parser.Parse(d)
	stages, _, err := instructions.Parse(r.AST)
	if err != nil {
		exit(err)
	}

	return stages, err
}
