package logger

import (
	"fmt"

	"github.com/fatih/color"
)

type Logger interface {
	Info(format string, label string, value string)
	Error(format string, label string, value string)
	Debug(format string, a ...interface{})
}

func Info(format string, label string, value string) {
	fmt.Printf(format+"\n", color.CyanString(label), color.WhiteString(value))
}

func Error(format string, label string, value string) {
	fmt.Printf(format+"\n", color.CyanString(label), color.RedString(value))
}

func Debug(format string, a ...interface{}) {
	b := color.New(color.FgGreen, color.BgBlack)
	b.Printf(format+"\n", a...)
}
