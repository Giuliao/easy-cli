package main

import (
	"fmt"
	"io"
	"os"

	"github.com/bytedance/easy-cli/internal/cli"
	"github.com/bytedance/easy-cli/internal/config"
	"github.com/bytedance/easy-cli/internal/skill"
	"github.com/bytedance/easy-cli/skills"
)

func main() {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	code := run(os.Args[1:], os.Stdout, os.Stderr, cwd, home)
	if code != 0 {
		os.Exit(code)
	}
}

func run(args []string, out, errOut io.Writer, workingDir, homeDir string) int {
	registry, err := skill.Load(skills.FS)
	if err != nil {
		fmt.Fprintf(errOut, "load embedded skills: %v\n", err)
		return 1
	}
	loadedConfig, configErr := config.Load(config.LoadOptions{WorkingDir: workingDir, HomeDir: homeDir})
	return cli.Run(args, registry, cli.Options{
		WorkingDir: workingDir,
		HomeDir:    homeDir,
		Config:     loadedConfig,
		ConfigErr:  configErr,
		In:         os.Stdin,
		Out:        out,
		ErrOut:     errOut,
	})
}
