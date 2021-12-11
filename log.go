package main

import (
	"io"
	"time"

	"github.com/fatih/color"
)

var (
	LogStream  = color.Output
	ErrStream  = color.Error
	TimeFormat = "2006/01/02 15:04:05"
)

func printLog(stream io.Writer, fg, bg color.Attribute, what, format string, args ...interface{}) {
	color.New(fg).Fprint(stream, time.Now().Format(TimeFormat))
	stream.Write([]byte(" "))
	color.New(bg).Fprint(stream, what)
	stream.Write([]byte(" "))
	color.New(fg).Fprintf(stream, format, args...)
	stream.Write([]byte("\n"))
}

func PrintLog(what, format string, args ...interface{}) {
	printLog(LogStream, color.Reset, color.Bold, what, format, args...)
}

func PrintImportant(what, format string, args ...interface{}) {
	printLog(LogStream, color.FgGreen, color.BgGreen, what, format, args...)
}

func PrintErr(what, format string, args ...interface{}) {
	printLog(ErrStream, color.FgRed, color.BgRed, what, format, args...)
}

func PrintWarn(what, format string, args ...interface{}) {
	printLog(ErrStream, color.FgYellow, color.BgYellow, what, format, args...)
}
