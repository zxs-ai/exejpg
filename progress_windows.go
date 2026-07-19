package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const progressScriptName = "progress.ps1"

type progressWindow struct {
	path       string
	totalFiles int
	totalBytes int64
	lastWrite  time.Time
	lastCopied int
	finished   bool
	mu         sync.Mutex
}

func installProgressScript(dstDir string) error {
	// Windows PowerShell 5 对无 BOM 的脚本可能按系统代码页读取，写 BOM 保证中文正常。
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(progressPowerShell)...)
	return os.WriteFile(filepath.Join(dstDir, progressScriptName), data, 0644)
}

func startProgressWindow(totalFiles int, totalBytes int64) *progressWindow {
	p := &progressWindow{
		path:       filepath.Join(os.TempDir(), fmt.Sprintf("CopyPairFolder_progress_%d.txt", os.Getpid())),
		totalFiles: totalFiles,
		totalBytes: totalBytes,
	}
	p.writeState(0, 0, "正在准备复制…")

	script := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyPairFolder", progressScriptName)
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden", "-File", script, p.path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
	return p
}

func (p *progressWindow) statusText(copied int, name string) string {
	if name != "" {
		return fmt.Sprintf("正在复制  共%d张，已复制%d张  %s", p.totalFiles, copied, name)
	}
	return fmt.Sprintf("正在复制  共%d张，已复制%d张", p.totalFiles, copied)
}

func (p *progressWindow) Update(copiedFiles int, copiedBytes int64, currentName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}

	// 张数变化必须立刻刷新；字节进度可节流
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
		p.writeState(100, copiedFiles, fmt.Sprintf("复制完成  共%d张，已复制%d张", p.totalFiles, copiedFiles))
		p.finished = true
	}
	p.mu.Unlock()
	// 等进度窗读到 100% 并自行关闭
	time.Sleep(1200 * time.Millisecond)
}

func (p *progressWindow) Close() {
	p.mu.Lock()
	// 无论是否已 Finish，都发关闭信号，避免窗口残留
	p.writeRaw("-1\n关闭\n" + strconv.Itoa(p.totalFiles) + "\n" + strconv.Itoa(p.lastCopied))
	p.finished = true
	p.mu.Unlock()
	time.Sleep(700 * time.Millisecond)
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

const progressPowerShell = `
param([string]$ProgressFile)
Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore

function Read-ProgressFile([string]$path) {
  $fs = $null
  $sr = $null
  try {
    $fs = [System.IO.File]::Open($path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
    $sr = New-Object System.IO.StreamReader($fs, [System.Text.Encoding]::UTF8)
    $text = $sr.ReadToEnd()
    if ([string]::IsNullOrWhiteSpace($text)) { return $null }
    return [regex]::Split($text, '\r?\n')
  } catch {
    return $null
  } finally {
    if ($sr) { $sr.Dispose() }
    if ($fs) { $fs.Dispose() }
  }
}

$window = New-Object System.Windows.Window
$window.Title = '复制并新建文件夹'
$window.Width = 520
$window.Height = 180
$window.ResizeMode = 'NoResize'
$window.WindowStartupLocation = 'CenterScreen'
$window.Topmost = $true

$grid = New-Object System.Windows.Controls.Grid
$grid.Margin = '22'
$row1 = New-Object System.Windows.Controls.RowDefinition
$row1.Height = 'Auto'
$row2 = New-Object System.Windows.Controls.RowDefinition
$row2.Height = '38'
$row3 = New-Object System.Windows.Controls.RowDefinition
$row3.Height = 'Auto'
$grid.RowDefinitions.Add($row1)
$grid.RowDefinitions.Add($row2)
$grid.RowDefinitions.Add($row3)

$title = New-Object System.Windows.Controls.TextBlock
$title.Text = '正在复制'
$title.FontSize = 16
$title.FontWeight = 'SemiBold'
$title.Margin = '0,0,0,8'
[System.Windows.Controls.Grid]::SetRow($title, 0)
$grid.Children.Add($title)

$bar = New-Object System.Windows.Controls.ProgressBar
$bar.Minimum = 0
$bar.Maximum = 100
$bar.Height = 22
$bar.Value = 0
[System.Windows.Controls.Grid]::SetRow($bar, 1)
$grid.Children.Add($bar)

$status = New-Object System.Windows.Controls.TextBlock
$status.Text = '正在准备复制…'
$status.FontSize = 13
$status.Margin = '0,10,0,0'
$status.TextTrimming = 'CharacterEllipsis'
[System.Windows.Controls.Grid]::SetRow($status, 2)
$grid.Children.Add($status)

$window.Content = $grid
$completedAt = $null
$seenProgress = $false
$timer = New-Object System.Windows.Threading.DispatcherTimer
$timer.Interval = [TimeSpan]::FromMilliseconds(80)
$timer.Add_Tick({
  try {
    if (-not (Test-Path -LiteralPath $ProgressFile)) {
      if ($seenProgress) {
        $timer.Stop()
        $window.Close()
      }
      return
    }

    $lines = Read-ProgressFile $ProgressFile
    if ($null -eq $lines -or $lines.Count -lt 1) { return }

    $value = 0
    if (-not [int]::TryParse($lines[0], [ref]$value)) { return }

    $seenProgress = $true
    if ($value -lt 0) {
      $timer.Stop()
      $window.Close()
      return
    }

    $bar.Value = [Math]::Min(100, [Math]::Max(0, $value))
    if ($lines.Count -ge 2 -and -not [string]::IsNullOrWhiteSpace($lines[1])) {
      $status.Text = $lines[1]
    }

    if ($value -ge 100) {
      $title.Text = '复制完成'
      if ($null -eq $completedAt) {
        $completedAt = [DateTime]::Now
      }
    }

    if ($null -ne $completedAt -and ([DateTime]::Now - $completedAt).TotalMilliseconds -ge 500) {
      $timer.Stop()
      $window.Close()
    }
  } catch {}
})
$window.Add_Closed({ $timer.Stop() })
$timer.Start()
[void]$window.ShowDialog()
`
