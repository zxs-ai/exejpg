//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func progressFinishDelay() time.Duration { return 200 * time.Millisecond }
func progressCloseDelay() time.Duration  { return 50 * time.Millisecond }

func startProgressWindow(totalFiles int, totalBytes int64) *progressWindow {
	p := &progressWindow{
		path:       filepath.Join(os.TempDir(), fmt.Sprintf("CopyPairFolder_progress_%d.txt", os.Getpid())),
		totalFiles: totalFiles,
		totalBytes: totalBytes,
	}
	p.writeState(0, 0, "正在准备复制…")
	_ = exec.Command("osascript", "-e",
		`display notification "正在复制配对文件…" with title "复制并新建文件夹"`).Start()
	return p
}
