package menu

import (
	"bufio"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xochilpili/subtitler-cli/internal/flags"
	"github.com/xochilpili/subtitler-cli/internal/logger"
	"github.com/xochilpili/subtitler-cli/internal/service"
)

type Menu struct {
	settings  *flags.OptionFlags
	service   service.Subdivx
	subtitles []service.Subtitles
}

func New(ctx context.Context, settings *flags.OptionFlags) *Menu {
	service := service.New(settings)
	subtitles := service.GetSubtitles(ctx)
	return &Menu{
		settings:  settings,
		service:   service,
		subtitles: subtitles,
	}
}

func (m *Menu) menu() {
	m.service.FormatSubtitles(m.subtitles)
}

func (m *Menu) Start() {
	m.start(os.Stdin)
}

func (m *Menu) start(reader io.Reader) {
	first := false
MainLoop:
	for {
		input := bufio.NewReader(reader)
		if !first {
			m.menu()
			first = true
		}

		logger.Info("%s, %s:", "Select a subtitle by index", "(q exit, m Subs)")
		inputString, err := input.ReadString('\n')
		if err != nil {
			break MainLoop
		}
		cmd, _ := cleanCommand(inputString)
		if len(cmd) < 1 {
			break MainLoop
		}
		// Route the first index of the cmd slice
	Route:
		switch cmd[0] {
		case "exit", "quit", "q":
			logger.Error("%v %v", "bye bye", "exiting...")
			break MainLoop
		case "menu", "m":
			m.menu()
		default:
			if _, err := strconv.Atoi(cmd[0]); err != nil {
				logger.Error("%v", "error:", "unrecognized option")
				break Route
			}
			index, _ := strconv.Atoi(cmd[0])
			if index > len(m.subtitles)-1 {
				logger.Error("%v", "error:", "invalid subtitle number")
				break Route
			}
			selected := m.subtitles[index : index+1][0]
			logger.Info("%v:%v", "Selected option", selected.Title)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			m.service.DownloadSubtitle(ctx, selected.Id)
			break MainLoop
		}
	}
}

func cleanCommand(cmd string) ([]string, error) {
	cmd_args := strings.Split(strings.Trim(cmd, "\r\n"), " ")
	return cmd_args, nil
}
