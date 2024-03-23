package main

import (
	"context"

	"github.com/xochilpili/subtitler-cli/internal/flags"
	"github.com/xochilpili/subtitler-cli/internal/menu"
)

func main() {
	ctx := context.Background()

	flags := flags.ParseFlags()

	primaryCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	m := menu.New(primaryCtx, flags)
	m.Start()
}
