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
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	exeName   = "CopyPairFolder.exe"
	mutexName = "Local\\CopyPairFolder_Leader_v5"
	regKeyName = "CopyPairFolder"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
	leaderMutex     windows.Handle
)

func helpText() string {
	return appName + "\n\n" +
		"双击本程序 = 安装右键菜单\n" +
		"卸载：CopyPairFolder.exe /uninstall\n\n" +
		"功能：仅复制「同名且同时存在 JPG 与 CR3」的配对到当前目录下的新文件夹。\n" +
		"扩展名大小写均可。"
}

func selectionSettleDelay() time.Duration { return 900 * time.Millisecond }

func messageBox(text, caption string) {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(caption)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40)
}

func doInstall() {
	dstDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyPairFolder")
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		messageBox("安装失败：无法创建目录\n"+err.Error(), appName)
		os.Exit(1)
	}

	self, err := os.Executable()
	if err != nil {
		messageBox("安装失败：无法定位自身\n"+err.Error(), appName)
		os.Exit(1)
	}
	self, _ = filepath.Abs(self)
	dstExe := filepath.Join(dstDir, exeName)

	if !sameFile(self, dstExe) {
		if err := copyFile(self, dstExe); err != nil {
			messageBox("安装失败：无法复制程序\n"+err.Error(), appName)
			os.Exit(1)
		}
	}

	if err := registerContextMenu(dstExe); err != nil {
		messageBox("安装失败：无法写入右键菜单\n"+err.Error(), appName)
		os.Exit(1)
	}
	if err := installProgressScript(dstDir); err != nil {
		messageBox("安装失败：无法安装进度窗口\n"+err.Error(), appName)
		os.Exit(1)
	}

	notifyShellAssocChanged()

	messageBox("安装成功！（兼容 Windows 10 / 11）\n\n"+
		"选中图片后右键 → 「"+menuCaption+"」\n\n"+
		"若 Win11 默认右键看不到：\n"+
		"· 点「显示更多选项」，或\n"+
		"· 按住 Shift 再右键\n\n"+
		"建议关闭并重新打开资源管理器窗口后再试。\n\n"+
		"卸载：\n"+dstExe+" /uninstall", appName)
}

func doUninstall() {
	_ = unregisterContextMenu()
	notifyShellAssocChanged()
	dstDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyPairFolder")
	_ = os.RemoveAll(dstDir)
	messageBox("已卸载右键菜单「"+menuCaption+"」。", appName)
}

func registerContextMenu(exePath string) error {
	_ = unregisterContextMenu()

	cmd := fmt.Sprintf(`"%s" "%%1"`, exePath)
	bases := []string{
		`HKCU\Software\Classes\*\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\image\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\.jpg\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\.jpeg\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\.jpe\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\.cr3\shell\` + regKeyName,
	}

	for _, base := range bases {
		cmds := [][]string{
			{"reg", "add", base, "/ve", "/d", menuCaption, "/f"},
			{"reg", "add", base, "/v", "Icon", "/d", exePath, "/f"},
			{"reg", "add", base, "/v", "MultiSelectModel", "/d", "Player", "/f"},
			{"reg", "add", base, "/v", "Position", "/d", "Top", "/f"},
			{"reg", "add", base + `\command`, "/ve", "/d", cmd, "/f"},
		}
		for _, c := range cmds {
			out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
			if err != nil {
				return fmt.Errorf("%v: %s", err, string(out))
			}
		}
	}
	return nil
}

func unregisterContextMenu() error {
	keys := []string{
		`HKCU\Software\Classes\*\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\image\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\.jpg\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\.jpeg\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\.jpe\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\.cr3\shell\` + regKeyName,
		`HKCU\Software\Classes\CLSID\` + comClassID,
		`HKCU\Software\Classes\AppID\` + comClassID,
	}
	for _, k := range keys {
		_ = exec.Command("reg", "delete", k, "/f").Run()
	}
	return nil
}

func notifyShellAssocChanged() {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("SHChangeNotify")
	const SHCNE_ASSOCCHANGED = 0x08000000
	const SHCNF_IDLIST = 0x0000
	_, _, _ = proc.Call(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, 0, 0)
}

func stateDir() string {
	d := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyPairFolder", "run")
	_ = os.MkdirAll(d, 0755)
	return d
}

func tryBecomeLeader(sessionFile string) bool {
	if st, err := os.Stat(sessionFile); err == nil {
		if time.Since(st.ModTime()) < sessionCooldown {
			return false
		}
		_ = os.Remove(sessionFile)
	}

	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return claimSessionFile(sessionFile)
	}
	h, err := windows.CreateMutex(nil, true, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(h)
		return false
	}
	if err != nil {
		return claimSessionFile(sessionFile)
	}

	if !claimSessionFile(sessionFile) {
		_ = windows.ReleaseMutex(h)
		_ = windows.CloseHandle(h)
		return false
	}

	leaderMutex = h
	return true
}

func getSelection(hint string) []string {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("cpf_sel_%d.txt", time.Now().UnixNano()))
	defer os.Remove(tmp)

	ps := `
$ErrorActionPreference = 'SilentlyContinue'
$hint = [System.IO.Path]::GetFullPath('` + escapePS(hint) + `')
$hintDir = [System.IO.Path]::GetDirectoryName($hint)
$outFile = '` + escapePS(tmp) + `'
$shell = New-Object -ComObject Shell.Application
$best = New-Object System.Collections.Generic.List[string]
$fallback = New-Object System.Collections.Generic.List[string]
foreach ($w in $shell.Windows()) {
  try {
    if ($null -eq $w.Document) { continue }
    $folderPath = $null
    try { $folderPath = $w.Document.Folder.Self.Path } catch {}
    $items = @($w.Document.SelectedItems())
    if ($items.Count -eq 0) { continue }
    $paths = New-Object System.Collections.Generic.List[string]
    foreach ($it in $items) {
      if ($it.Path) { [void]$paths.Add([string]$it.Path) }
    }
    if ($paths.Count -eq 0) { continue }
    $fallback = $paths
    $hit = $false
    foreach ($p in $paths) {
      if ([string]::Equals($p, $hint, [System.StringComparison]::OrdinalIgnoreCase)) { $hit = $true; break }
    }
    if (-not $hit -and $folderPath -and $hintDir -and [string]::Equals($folderPath, $hintDir, [System.StringComparison]::OrdinalIgnoreCase)) {
      $hit = $true
    }
    if ($hit) { $best = $paths; break }
  } catch {}
}
$result = $best
if ($result.Count -eq 0) { $result = $fallback }
$utf8 = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllLines($outFile, $result.ToArray(), $utf8)
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()

	data, err := os.ReadFile(tmp)
	if err != nil {
		return nil
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	var res []string
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(strings.TrimRight(ln, "\r"))
		if ln != "" {
			res = append(res, ln)
		}
	}
	return res
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func openAndRenameFolder(folder string) {
	parent := filepath.Dir(folder)
	_ = exec.Command("explorer", "/select,", folder).Start()

	ps := `
Start-Sleep -Milliseconds 900
$folder = '` + escapePS(folder) + `'
$parent = '` + escapePS(parent) + `'
$shell = New-Object -ComObject Shell.Application
foreach ($w in $shell.Windows()) {
  try {
    if ($w.Document.Folder.Self.Path -eq $parent) {
      $w.Document.SelectItem($folder, 1+4+8+16)
      Start-Sleep -Milliseconds 200
      $wshell = New-Object -ComObject WScript.Shell
      $wshell.AppActivate($w.HWND) | Out-Null
      Start-Sleep -Milliseconds 200
      $wshell.SendKeys('{F2}')
      break
    }
  } catch {}
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
}
