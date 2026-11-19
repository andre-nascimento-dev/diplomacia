package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
void startTray(void);
*/
import "C"

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var trayCtx context.Context

//export trayOpen
func trayOpen() { runtime.WindowShow(trayCtx) }

// trayQuit passa pelo Quit do Wails para o OnShutdown desconectar o WhatsApp.
//
//export trayQuit
func trayQuit() { runtime.Quit(trayCtx) }

func startTray(ctx context.Context) {
	trayCtx = ctx
	C.startTray()
}
