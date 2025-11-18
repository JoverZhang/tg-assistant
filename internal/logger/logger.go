package logger

import (
	"log"
	"os"

	"github.com/fatih/color"
)

var (
	Info  = log.New(os.Stdout, color.GreenString("[INFO] "), log.LstdFlags|log.Lmsgprefix)
	Warn  = log.New(os.Stdout, color.YellowString("[WARN] "), log.LstdFlags|log.Lmsgprefix)
	Error = log.New(os.Stderr, color.RedString("[ERROR] "), log.LstdFlags|log.Lmsgprefix)
	Debug = log.New(os.Stdout, color.CyanString("[DEBUG] "), log.LstdFlags|log.Lmsgprefix)
	// Disable Debug logging
	// Debug = log.New(io.Discard, "", 0)
)
