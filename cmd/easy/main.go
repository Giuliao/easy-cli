package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Giuliao/easy-cli/internal/cli"
	"github.com/Giuliao/easy-cli/internal/config"
	"github.com/Giuliao/easy-cli/internal/skill"
	"github.com/Giuliao/easy-cli/skills"
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
	registry, err := skill.LoadAll(skills.FS, skill.DiscoveryOptions{
		WorkingDir: workingDir,
		HomeDir:    homeDir,
	})
	if err != nil {
		fmt.Fprintf(errOut, "load skills: %v\n", err)
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
