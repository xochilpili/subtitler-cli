package flags

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jedib0t/go-pretty/v6/table"
)

type Releases []string

func (r *Releases) String() string {
	return fmt.Sprintln(*r)
}
func (r *Releases) Set(s string) error {
	*r = append(*r, s)
	return nil
}

type OptionFlags struct {
	Title        string
	Releases     []string
	Debug        bool
	Style        table.Style
	DownloadPath string
}

func ParseFlags() *OptionFlags {
	// TODO: Add a download path
	titleFlag := flag.String("s", "", "Subtitle title")
	var releases Releases
	flag.Var(&releases, "r", "Releases")
	debug := flag.Bool("d", false, "Debug mode")
	style := flag.String("t", "dark", "table style")
	downloadPath := flag.String("p", ".", "Download path")

	flag.Parse()
	if len(*titleFlag) <= 0 {
		panic("title is required flag")
	}

	if _, err := os.Stat(*downloadPath); os.IsNotExist(err) {
		panic("download path does not exist")
	}

	dirname, _ := filepath.Abs(*downloadPath)

	var selectedStyle table.Style
	switch *style {
	case "dark":
		selectedStyle = table.StyleColoredDark
	case "light":
		selectedStyle = table.StyleLight
	case "bright":
		selectedStyle = table.StyleColoredBright
	case "white":
		selectedStyle = table.StyleColoredBlackOnGreenWhite
	case "red":
		selectedStyle = table.StyleColoredBlackOnRedWhite
	}

	return &OptionFlags{
		Title:        *titleFlag,
		Releases:     releases,
		Debug:        *debug,
		Style:        selectedStyle,
		DownloadPath: dirname,
	}
}
