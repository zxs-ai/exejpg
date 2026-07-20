package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type progressWindow struct {
	path       string
	totalFiles int
	totalBytes int64
	mode       transferMode
	lastWrite  time.Time
	lastCopied int
	finished   bool
	mu         sync.Mutex
}

func (p *progressWindow) statusText(copied int, name string) string {
	verb := p.mode.verb()
	if name != "" {
		return fmt.Sprintf("正在%s  共%d张，已完成%d张  %s", verb, p.totalFiles, copied, name)
	}
	return fmt.Sprintf("正在%s  共%d张，已完成%d张", verb, p.totalFiles, copied)
}

func (p *progressWindow) Update(copiedFiles int, copiedBytes int64, currentName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}

	force := copiedFiles != p.lastCopied
	if !force && time.Since(p.lastWrite) < 80*time.Millisecond {
		return
	}

	percent := 0
	if p.totalBytes > 0 {
		percent = int(copiedBytes * 100 / p.totalBytes)
	} else if p.totalFiles > 0 {
		percent = copiedFiles * 100 / p.totalFiles
	}
	if percent > 99 {
		percent = 99
	}
	p.lastCopied = copiedFiles
	p.writeState(percent, copiedFiles, p.statusText(copiedFiles, currentName))
}

func (p *progressWindow) Finish(copiedFiles int) {
	p.mu.Lock()
	if !p.finished {
		p.lastCopied = copiedFiles
		p.writeState(100, copiedFiles, fmt.Sprintf("%s完成  共%d张，已完成%d张", p.mode.verb(), p.totalFiles, copiedFiles))
		p.finished = true
	}
	p.mu.Unlock()
	time.Sleep(progressFinishDelay())
}

func (p *progressWindow) Close() {
	p.mu.Lock()
	p.writeRaw("-1\n关闭\n" + strconv.Itoa(p.totalFiles) + "\n" + strconv.Itoa(p.lastCopied))
	p.finished = true
	p.mu.Unlock()
	time.Sleep(progressCloseDelay())
	_ = os.Remove(p.path)
}

func (p *progressWindow) writeState(percent, copied int, status string) {
	status = strings.ReplaceAll(status, "\r", " ")
	status = strings.ReplaceAll(status, "\n", " ")
	content := fmt.Sprintf("%d\n%s\n%d\n%d", percent, status, p.totalFiles, copied)
	p.writeRaw(content)
}

func (p *progressWindow) writeRaw(content string) {
	_ = os.WriteFile(p.path, []byte(content), 0644)
	p.lastWrite = time.Now()
}
