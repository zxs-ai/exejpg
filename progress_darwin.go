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

func startProgressWindow(totalFiles int, totalBytes int64, mode transferMode) *progressWindow {
	p := &progressWindow{
		path:       filepath.Join(os.TempDir(), fmt.Sprintf("CopyPairFolder_progress_%d.txt", os.Getpid())),
		totalFiles: totalFiles,
		totalBytes: totalBytes,
		mode:       mode,
	}
	p.writeState(0, 0, "正在准备"+mode.verb()+"…")
	script := fmt.Sprintf(`display notification %s with title %s`,
		appleScriptString("正在"+mode.verb()+"配对文件…"),
		appleScriptString(mode.menuCaption()))
	_ = exec.Command("osascript", "-e", script).Start()
	return p
}
