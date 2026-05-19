package rime

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gaboolic/moqi-ime/imecore"
)

var (
	sharedAppearanceSkinImportState struct {
		mu      sync.Mutex
		running bool
	}
	appearanceSkinImportPromptFunc = promptAppearanceSkinFile
)

func resetAppearanceSkinImportStateForTest() {
	sharedAppearanceSkinImportState.mu.Lock()
	sharedAppearanceSkinImportState.running = false
	sharedAppearanceSkinImportState.mu.Unlock()
	appearanceSkinImportPromptFunc = promptAppearanceSkinFile
}

func (ime *IME) importAppearanceSkinAsync(resp *imecore.Response) bool {
	sharedAppearanceSkinImportState.mu.Lock()
	if sharedAppearanceSkinImportState.running {
		sharedAppearanceSkinImportState.mu.Unlock()
		if resp != nil {
			resp.TrayNotification = trayNotification("皮肤导入已在进行中", imecore.TrayNotificationIconInfo)
		}
		return false
	}
	sharedAppearanceSkinImportState.running = true
	sharedAppearanceSkinImportState.mu.Unlock()

	if resp != nil {
		resp.TrayNotification = trayNotification("请选择要导入的皮肤文件...", imecore.TrayNotificationIconInfo)
	}

	go func() {
		defer func() {
			sharedAppearanceSkinImportState.mu.Lock()
			sharedAppearanceSkinImportState.running = false
			sharedAppearanceSkinImportState.mu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		path, err := appearanceSkinImportPromptFunc(ctx)
		if err != nil {
			log.Printf("导入皮肤失败: %v", err)
			ime.sendAsyncTrayNotification(trayNotification("导入皮肤失败: "+shortErrorMessage(err), imecore.TrayNotificationIconError))
			return
		}
		if path == "" {
			return
		}

		ime.mu.Lock()
		defer ime.mu.Unlock()
		if err := ime.importAppearanceSkinFromFile(path); err != nil {
			log.Printf("导入皮肤失败: %v", err)
			ime.sendAsyncTrayNotification(trayNotification("导入皮肤失败: "+shortErrorMessage(err), imecore.TrayNotificationIconError))
			return
		}
		ime.sendAsyncAppearanceUpdate(trayNotification("皮肤已导入并立即生效", imecore.TrayNotificationIconInfo))
	}()

	return true
}
