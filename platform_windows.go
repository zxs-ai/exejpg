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
	"golang.org/x/sys/windows/registry"
)

const (
	exeName    = "CopyPairFolder.exe"
	regKeyCopy = "CopyPairFolder"
	regKeyCut  = "CutPairFolder"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
	leaderMutexes   = map[transferMode]windows.Handle{}
)

func helpText() string {
	return appName + "\n\n" +
		"双击本程序 = 安装右键菜单\n" +
		"卸载：CopyPairFolder.exe /uninstall\n\n" +
		"右键菜单两项：\n" +
		"· 「" + menuCopyCaption + "」— 配对复制到新文件夹，源文件保留\n" +
		"· 「" + menuCutCaption + "」— 配对剪切到新文件夹，源文件删除\n\n" +
		"仅处理「同名且同时存在 JPG 与 CR3」的配对；扩展名大小写均可。"
}

func selectionSettleDelay() time.Duration { return 900 * time.Millisecond }

func messageBox(text, caption string) {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(caption)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40)
}

func confirmDialog(text, caption string) bool {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(caption)
	const mbYesNo = 0x04
	const mbIconQuestion = 0x20
	const idYes = 6
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), mbYesNo|mbIconQuestion)
	return r == idYes
}

func doInstall() {
	closeBusy := showInstallBusyWindow("正在安装，请稍后…")
	fail := func(msg string) {
		closeBusy()
		messageBox(msg, appName)
		os.Exit(1)
	}

	dstDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyPairFolder")
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		fail("安装失败：无法创建目录\n" + err.Error())
	}

	self, err := os.Executable()
	if err != nil {
		fail("安装失败：无法定位自身\n" + err.Error())
	}
	self, _ = filepath.Abs(self)
	dstExe := filepath.Join(dstDir, exeName)

	if !sameFile(self, dstExe) {
		if err := copyFile(self, dstExe); err != nil {
			fail("安装失败：无法复制程序\n" + err.Error())
		}
	}

	if err := registerContextMenu(dstExe); err != nil {
		fail("安装失败：无法写入右键菜单\n" + err.Error())
	}
	if err := installProgressScript(dstDir); err != nil {
		fail("安装失败：无法安装进度窗口\n" + err.Error())
	}

	notifyShellAssocChanged()
	closeBusy()

	messageBox("安装成功！（兼容 Windows 10 / 11）\n\n"+
		"安装位置：\n"+dstDir+"\n\n"+
		"怎么用：\n"+
		"1. 关闭并重新打开资源管理器窗口\n"+
		"2. 选中 JPG / CR3 后右键，可见：\n"+
		"   · 「"+menuCopyCaption+"」— 复制到新文件夹\n"+
		"   · 「"+menuCutCaption+"」— 剪切到新文件夹\n\n"+
		"若 Win11 默认右键看不到：\n"+
		"· 点「显示更多选项」，或按住 Shift 再右键\n\n"+
		"怎么卸载：\n"+
		dstExe+" /uninstall\n\n"+
		"点「确定」关闭本提示。", appName)
}

func doUninstall() {
	closeBusy := showInstallBusyWindow("正在卸载，请稍后…")
	_ = unregisterContextMenu()
	notifyShellAssocChanged()
	dstDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyPairFolder")
	_ = os.RemoveAll(dstDir)
	closeBusy()
	messageBox("已卸载。\n\n右键菜单「"+menuCopyCaption+"」「"+menuCutCaption+"」已移除。\n点「确定」关闭。", appName)
}

func shellMenuBases() []string {
	return []string{
		`Software\Classes\*\shell\`,
		`Software\Classes\SystemFileAssociations\image\shell\`,
		`Software\Classes\SystemFileAssociations\.jpg\shell\`,
		`Software\Classes\SystemFileAssociations\.jpeg\shell\`,
		`Software\Classes\SystemFileAssociations\.jpe\shell\`,
		`Software\Classes\SystemFileAssociations\.cr3\shell\`,
	}
}

func registerContextMenu(exePath string) error {
	_ = unregisterContextMenu()

	type menuDef struct {
		key     string
		caption string
		flag    string
	}
	menus := []menuDef{
		{regKeyCopy, menuCopyCaption, "/copy"},
		{regKeyCut, menuCutCaption, "/cut"},
	}

	for _, m := range menus {
		cmd := fmt.Sprintf(`"%s" %s "%%1"`, exePath, m.flag)
		for _, prefix := range shellMenuBases() {
			base := prefix + m.key
			if err := regWriteShellMenu(base, m.caption, exePath, cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

func regWriteShellMenu(keyPath, caption, icon, command string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("", caption); err != nil {
		return err
	}
	if err := k.SetStringValue("Icon", icon); err != nil {
		return err
	}
	if err := k.SetStringValue("MultiSelectModel", "Player"); err != nil {
		return err
	}
	if err := k.SetStringValue("Position", "Top"); err != nil {
		return err
	}

	ck, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath+`\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer ck.Close()
	return ck.SetStringValue("", command)
}

func unregisterContextMenu() error {
	for _, prefix := range shellMenuBases() {
		deleteRegTree(prefix + regKeyCopy)
		deleteRegTree(prefix + regKeyCut)
	}
	deleteRegTree(`Software\Classes\CLSID\` + comClassID)
	deleteRegTree(`Software\Classes\AppID\` + comClassID)
	return nil
}

func deleteRegTree(keyPath string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return
	}
	names, _ := k.ReadSubKeyNames(-1)
	_ = k.Close()
	for _, name := range names {
		deleteRegTree(keyPath + `\` + name)
	}
	_ = registry.DeleteKey(registry.CURRENT_USER, keyPath)
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

func mutexNameFor(mode transferMode) string {
	return "Local\\CopyPairFolder_Leader_v6_" + mode.String()
}

func tryBecomeLeader(sessionFile string, mode transferMode) bool {
	if st, err := os.Stat(sessionFile); err == nil {
		if time.Since(st.ModTime()) < sessionCooldown {
			return false
		}
		_ = os.Remove(sessionFile)
	}

	name, err := windows.UTF16PtrFromString(mutexNameFor(mode))
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

	leaderMutexes[mode] = h
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
	cmdExp := exec.Command("explorer", "/select,", folder)
	cmdExp.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmdExp.Start()

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
