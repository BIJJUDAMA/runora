package model

import (
	"os"
	"time"
)

// ReplaceFile replaces dst with src atomically with a retry loop to handle transient locks (especially on Windows).
func ReplaceFile(src, dst string) error {
	var err error
	for i := 0; i < 10; i++ {
		err = os.Rename(src, dst)
		if err == nil {
			return nil
		}
		// If rename failed due to destination existing or permission/lock, attempt removal and retry
		if os.IsExist(err) || os.IsPermission(err) {
			_ = os.Remove(dst)
		}
		time.Sleep(time.Duration(25*(i+1)) * time.Millisecond)
	}
	return err
}
