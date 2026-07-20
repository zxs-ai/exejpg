//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func startProgressWindow(totalFiles int, totalBytes int64, mode transferMode) *progressWindow {
	p := &progressWindow{
		path:       filepath.Join(os.TempDir(), fmt.Sprintf("CopyPairFolder_progress_%d.txt", os.Getpid())),
		totalFiles: totalFiles,
		totalBytes: totalBytes,
		mode:       mode,
	}
	p.writeState(0, 0, "正在准备"+mode.verb()+"…")

	script := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyPairFolder", progressScriptName)
	title := mode.menuCaption()
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden", "-File", script, p.path, title, mode.verb())
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
	return p
}

// 安装/卸载期间只显示这一个忙碌窗，避免 reg.exe 等控制台狂闪
func showInstallBusyWindow(status string) func() {
	statePath := filepath.Join(os.TempDir(), fmt.Sprintf("CopyPairFolder_install_%d.txt", os.Getpid()))
	_ = os.WriteFile(statePath, []byte("0\n"+status), 0644)

	ps1 := filepath.Join(os.TempDir(), fmt.Sprintf("CopyPairFolder_install_%d.ps1", os.Getpid()))
	script := strings.ReplaceAll(installBusyPowerShell, "\n", "\r\n")
	_ = os.WriteFile(ps1, append([]byte{0xEF, 0xBB, 0xBF}, []byte(script)...), 0644)

	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden", "-File", ps1, statePath, appName, status)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()

	// 给窗口一点启动时间
	time.Sleep(250 * time.Millisecond)

	return func() {
		_ = os.WriteFile(statePath, []byte("-1\n关闭"), 0644)
		time.Sleep(500 * time.Millisecond)
		_ = os.Remove(statePath)
		_ = os.Remove(ps1)
	}
}

const installBusyPowerShell = `
param(
  [string]$StateFile,
  [string]$WindowTitle = '配对文件夹工具',
  [string]$StatusText = '正在安装，请稍后…'
)
Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore

function Read-State([string]$path) {
  try {
    $fs = [System.IO.File]::Open($path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
    $sr = New-Object System.IO.StreamReader($fs, [System.Text.Encoding]::UTF8)
    $text = $sr.ReadToEnd()
    $sr.Dispose(); $fs.Dispose()
    if ([string]::IsNullOrWhiteSpace($text)) { return $null }
    return [regex]::Split($text, '\r?\n')
  } catch { return $null }
}

$window = New-Object System.Windows.Window
$window.Title = $WindowTitle
$window.Width = 420
$window.Height = 160
$window.ResizeMode = 'NoResize'
$window.WindowStartupLocation = 'CenterScreen'
$window.Topmost = $true
$window.WindowStyle = 'ToolWindow'

$grid = New-Object System.Windows.Controls.Grid
$grid.Margin = '24'
$row1 = New-Object System.Windows.Controls.RowDefinition
$row1.Height = 'Auto'
$row2 = New-Object System.Windows.Controls.RowDefinition
$row2.Height = '28'
$row3 = New-Object System.Windows.Controls.RowDefinition
$row3.Height = 'Auto'
$grid.RowDefinitions.Add($row1)
$grid.RowDefinitions.Add($row2)
$grid.RowDefinitions.Add($row3)

$title = New-Object System.Windows.Controls.TextBlock
$title.Text = '请稍候'
$title.FontSize = 16
$title.FontWeight = 'SemiBold'
$title.Margin = '0,0,0,10'
[System.Windows.Controls.Grid]::SetRow($title, 0)
$grid.Children.Add($title)

$bar = New-Object System.Windows.Controls.ProgressBar
$bar.IsIndeterminate = $true
$bar.Height = 18
[System.Windows.Controls.Grid]::SetRow($bar, 1)
$grid.Children.Add($bar)

$status = New-Object System.Windows.Controls.TextBlock
$status.Text = $StatusText
$status.FontSize = 13
$status.Margin = '0,12,0,0'
[System.Windows.Controls.Grid]::SetRow($status, 2)
$grid.Children.Add($status)

$window.Content = $grid
$timer = New-Object System.Windows.Threading.DispatcherTimer
$timer.Interval = [TimeSpan]::FromMilliseconds(120)
$timer.Add_Tick({
  try {
    if (-not (Test-Path -LiteralPath $StateFile)) {
      $timer.Stop(); $window.Close(); return
    }
    $lines = Read-State $StateFile
    if ($null -eq $lines -or $lines.Count -lt 1) { return }
    $value = 0
    if (-not [int]::TryParse($lines[0], [ref]$value)) { return }
    if ($value -lt 0) { $timer.Stop(); $window.Close(); return }
    if ($lines.Count -ge 2 -and -not [string]::IsNullOrWhiteSpace($lines[1])) {
      $status.Text = $lines[1]
    }
  } catch {}
})
$window.Add_Closed({ $timer.Stop() })
$timer.Start()
[void]$window.ShowDialog()
`

const progressPowerShell = `
param(
  [string]$ProgressFile,
  [string]$WindowTitle = '配对文件夹工具',
  [string]$Verb = '复制'
)
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
$window.Title = $WindowTitle
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
$title.Text = "正在$Verb"
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
$status.Text = "正在准备$Verb…"
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
      $title.Text = "$Verb完成"
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
