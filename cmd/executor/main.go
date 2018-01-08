package main

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/dlorenc/kbuild/pkg/dockerfile"
	"github.com/dlorenc/kbuild/pkg/snapshot"
	"github.com/docker/docker/builder/dockerfile/instructions"
)

var dockerfilePath = flag.String("dockerfile", "/dockerfile/Dockerfile", "Path to Dockerfile.")

func main() {
	flag.Parse()

	b, err := ioutil.ReadFile(*dockerfilePath)
	if err != nil {
		panic(err)
	}

	stages, err := dockerfile.Parse(b)
	if err != nil {
		panic(err)
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
		fi, err := os.Lstat(p)
		if err != nil {
			panic(err)
		}
		h.Write([]byte(fi.Mode().String()))
		h.Write([]byte(fi.ModTime().String()))

		if fi.Mode().IsRegular() {
			f, err := os.Open(p)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			if _, err := io.Copy(h, f); err != nil {
				panic(err)
			}
		}

		return hex.EncodeToString(h.Sum(nil))
	}

	l := snapshot.NewLayeredMap(hasher)
	snapshotter := snapshot.NewSnapshotter(l, "/work-dir")

	// Take initial snapshot
	if err := snapshotter.Init(); err != nil {
		panic(err)
	}

	for _, c := range commandsToRun {
		fmt.Println("cmd: ", c[0])
		fmt.Println("args: ", c[1:])
		if err != nil {
			panic(err)
		}
		cmd := exec.Command(c[0], c[1:]...)
		combout, err := cmd.CombinedOutput()
		if err != nil {
			panic(err)
		}
		fmt.Printf("Output from %s %s\n", cmd.Path, cmd.Args)
		fmt.Print(string(combout))

		if err := snapshotter.TakeSnapshot(); err != nil {
			panic(err)
		}
	}
}
