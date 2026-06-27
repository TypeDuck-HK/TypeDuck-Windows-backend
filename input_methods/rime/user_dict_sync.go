package rime

import (
	"fmt"
	"sync"

	"github.com/gaboolic/moqi-ime/imecore"
)

type userDictSyncState struct {
	mu      sync.Mutex
	running bool
}

var sharedUserDictSyncState userDictSyncState

func resetUserDictSyncStateForTest() {
	sharedUserDictSyncState.mu.Lock()
	sharedUserDictSyncState.running = false
	sharedUserDictSyncState.mu.Unlock()
}

func (s *userDictSyncState) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *userDictSyncState) end() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

func (ime *IME) syncUserDataCommand(resp *imecore.Response) bool {
	if ime.backend == nil {
		if resp != nil {
			resp.TrayNotification = trayNotification("User dictionary sync failed: Rime backend is unavailable", imecore.TrayNotificationIconError)
		}
		return false
	}
	if !sharedUserDictSyncState.begin() {
		if resp != nil {
			resp.TrayNotification = trayNotification("User dictionary sync is already running", imecore.TrayNotificationIconInfo)
			resp.ReturnValue = 1
		}
		return true
	}

	if ime.asyncResponseSender == nil {
		defer sharedUserDictSyncState.end()
		err := ime.syncUserDataLocal()
		if resp != nil {
			resp.TrayNotification = userDictSyncTrayNotification(err)
		}
		return err == nil
	}

	if resp != nil {
		resp.TrayNotification = trayNotification("Starting user dictionary sync...", imecore.TrayNotificationIconInfo)
		resp.ReturnValue = 1
	}
	go func() {
		defer sharedUserDictSyncState.end()
		err := ime.syncUserDataLocal()
		ime.sendAsyncTrayNotification(userDictSyncTrayNotification(err))
	}()
	return true
}

func (ime *IME) syncUserDataLocal() error {
	if ime.backend == nil || !ime.backend.SyncUserData() {
		return fmt.Errorf("librime user dictionary merge failed")
	}
	return nil
}

func userDictSyncTrayNotification(err error) *imecore.TrayNotification {
	if err != nil {
		return trayNotification("User dictionary sync failed: "+shortErrorMessage(err), imecore.TrayNotificationIconError)
	}
	return trayNotification("User dictionary sync completed", imecore.TrayNotificationIconInfo)
}
