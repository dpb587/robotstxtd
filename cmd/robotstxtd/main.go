package main

import (
	"github.com/dpb587/robotstxtd/cmd/robotstxtd/rpccmd"
	"github.com/dpb587/robotstxtd/cmd/robotstxtd/servecmd"
	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use: "robotstxtd",
	}

	cmd.AddCommand(rpccmd.NewCommand())
	cmd.AddCommand(servecmd.NewCommand())

	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
