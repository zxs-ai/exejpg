//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const progressScriptName = "progress.ps1"

func progressFinishDelay() time.Duration { return 1200 * time.Millisecond }
func progressCloseDelay() time.Duration  { return 700 * time.Millisecond }

func installProgressScript(dstDir string) error {
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
