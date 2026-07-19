package main

import (
	"fmt"
	"io"
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
	appName     = "复制并新建文件夹"
	exeName     = "CopyPairFolder.exe"
	regKeyName  = "CopyPairFolder"
	menuCaption = "复制并新建文件夹"
	mutexName   = "Local\\CopyPairFolder_Leader_v5"
	// 资源管理器多选时常按文件逐个启动；同一操作窗口期内只允许跑一次
	sessionCooldown = 8 * time.Second
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
	leaderMutex     windows.Handle // 进程存活期间持有，防止后续实例再成为领导
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		doInstall()
		return
	}

	switch strings.ToLower(args[0]) {
	case "/install", "-install", "--install":
		doInstall()
	case "/uninstall", "-uninstall", "--uninstall":
		doUninstall()
	case "/embedding", "-embedding":
		runCOMServer()
	case "/process-list":
		if len(args) >= 2 {
			runCopyFromList(args[1])
		}
	case "/help", "-h", "--help":
		showHelp()
	default:
		runCopy(args)
	}
}

func showHelp() {
	msg := appName + "\n\n" +
		"双击本程序 = 安装右键菜单\n" +
		"卸载：CopyPairFolder.exe /uninstall\n\n" +
		"功能：仅复制「同名且同时存在 JPG 与 CR3」的配对到当前目录下的新文件夹。\n" +
		"扩展名大小写均可。"
	messageBox(msg, appName)
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

	messageBox("安装成功！\n\n选中任意数量的图片后右键可见：「"+menuCaption+"」\n\n同名 JPG+CR3 才复制；处理时显示进度条。\n\n卸载：\n"+dstExe+" /uninstall", appName)
}

func doUninstall() {
	_ = unregisterContextMenu()
	dstDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyPairFolder")
	_ = os.RemoveAll(dstDir)
	messageBox("已卸载右键菜单「"+menuCaption+"」。", appName)
}

func registerContextMenu(exePath string) error {
	_ = unregisterContextMenu()

	base := `HKCU\Software\Classes\*\shell\` + regKeyName
	clsidKey := `HKCU\Software\Classes\CLSID\` + comClassID
	localServer := fmt.Sprintf(`"%s" /Embedding`, exePath)

	cmds := [][]string{
		{"reg", "add", base, "/ve", "/d", menuCaption, "/f"},
		{"reg", "add", base, "/v", "Icon", "/d", exePath, "/f"},
		{"reg", "add", base, "/v", "MultiSelectModel", "/d", "Player", "/f"},
		{"reg", "add", base + `\command`, "/v", "DelegateExecute", "/d", comClassID, "/f"},
		{"reg", "add", clsidKey, "/ve", "/d", appName, "/f"},
		{"reg", "add", clsidKey + `\LocalServer32`, "/ve", "/d", localServer, "/f"},
		{"reg", "add", clsidKey + `\LocalServer32`, "/v", "ServerExecutable", "/d", exePath, "/f"},
	}
	for _, c := range cmds {
		out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, string(out))
		}
	}
	return nil
}

func unregisterContextMenu() error {
	keys := []string{
		`HKCU\Software\Classes\*\shell\` + regKeyName,
		`HKCU\Software\Classes\SystemFileAssociations\image\shell\` + regKeyName,
		`HKCU\Software\Classes\CLSID\` + comClassID,
	}
	for _, k := range keys {
		_ = exec.Command("reg", "delete", k, "/f").Run()
	}
	return nil
}

func stateDir() string {
	d := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyPairFolder", "run")
	_ = os.MkdirAll(d, 0755)
	return d
}

func runCopy(args []string) {
	clicked := strings.TrimSpace(args[0])
	if clicked == "" {
		os.Exit(0)
	}
	clicked, _ = filepath.Abs(clicked)

	listFile := filepath.Join(stateDir(), "pending_files.txt")
	sessionFile := filepath.Join(stateDir(), "session.active")

	// 先记下本进程收到的文件（静默）
	appendPending(listFile, clicked)

	// 同一轮右键操作里，后续被逐个拉起的进程全部静默退出，绝不弹窗
	if !tryBecomeLeader(sessionFile) {
		os.Exit(0)
	}
	// 领导进程：保持 session 文件到结束（含弹窗期间），防止逐个启动的后续进程再跑一遍
	defer touchSession(sessionFile)

	// 等并行启动的进程写完 pending；并给资源管理器一点时间保持多选状态
	time.Sleep(600 * time.Millisecond)

	selected := getExplorerSelection(clicked)
	selected = append(selected, readPending(listFile)...)
	selected = append(selected, collectArgsFiles(args)...)
	_ = os.Remove(listFile)

	files := normalizeFiles(selected)
	processSelected(files)
}

func runCopyFromList(listPath string) {
	defer os.Remove(listPath)
	data, err := os.ReadFile(listPath)
	if err != nil {
		messageBox("读取选中文件失败：\n"+err.Error(), appName)
		return
	}
	var selected []string
	for _, line := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		if line = strings.TrimSpace(strings.TrimRight(line, "\r")); line != "" {
			selected = append(selected, line)
		}
	}
	processSelected(normalizeFiles(selected))
}

func processSelected(files []string) {
	if len(files) == 0 {
		messageBox("未获取到选中的文件。\n\n请重新选中后右键再试。", appName)
		return
	}

	dir := filepath.Dir(files[0])
	pairs := findJpgCr3Pairs(files, dir)
	if len(pairs) == 0 {
		messageBox("未找到同时具备 JPG 与 CR3 的同名配对。\n\n只有一种格式的文件不会复制。", appName)
		return
	}

	folderName := "配对导出_" + time.Now().Format("20060102_150405")
	destDir := uniqueDir(filepath.Join(dir, folderName))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		messageBox("创建文件夹失败：\n"+err.Error(), appName)
		return
	}

	totalBytes := int64(0)
	for _, paths := range pairs {
		for _, src := range paths {
			if st, err := os.Stat(src); err == nil {
				totalBytes += st.Size()
			}
		}
	}
	totalCopy := len(pairs) * 2
	progress := startProgressWindow(totalCopy, totalBytes)
	defer progress.Close()

	copied := 0
	var copiedBytes int64
	for _, paths := range pairs {
		for _, src := range paths {
			dst := filepath.Join(destDir, filepath.Base(src))
			name := filepath.Base(src)
			progress.Update(copied, copiedBytes, name)
			n, err := copyFileWithProgress(src, dst, func(delta int64) {
				copiedBytes += delta
				progress.Update(copied, copiedBytes, name)
			})
			if err != nil {
				messageBox("复制失败：\n"+err.Error(), appName)
				return
			}
			_ = n
			copied++
			progress.Update(copied, copiedBytes, "")
		}
	}

	progress.Finish(copied)
	openAndRenameFolder(destDir)
	messageBox(fmt.Sprintf("完成！\n\n配对组数：%d\n复制文件：%d\n新文件夹：%s",
		len(pairs), copied, destDir), appName)
}

// tryBecomeLeader：整轮操作只允许一个进程执行并弹窗。
// 覆盖两种情况：1) 同时启动多个  2) 等上一个退出后再启动下一个
func tryBecomeLeader(sessionFile string) bool {
	// 冷却期内已有会话 → 静默退出（解决「按文件逐个启动」）
	if st, err := os.Stat(sessionFile); err == nil {
		if time.Since(st.ModTime()) < sessionCooldown {
			return false
		}
		_ = os.Remove(sessionFile)
	}

	// 命名互斥量：并发时只有创建者成为领导
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

	// 创建成功并拥有所有权 → 再占 session 文件
	if !claimSessionFile(sessionFile) {
		_ = windows.ReleaseMutex(h)
		_ = windows.CloseHandle(h)
		return false
	}

	// 弹窗期间也持有，进程退出时由系统回收
	leaderMutex = h
	return true
}

func claimSessionFile(sessionFile string) bool {
	if st, err := os.Stat(sessionFile); err == nil {
		if time.Since(st.ModTime()) < sessionCooldown {
			return false
		}
		_ = os.Remove(sessionFile)
	}
	f, err := os.OpenFile(sessionFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return false
	}
	_, _ = f.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
	_ = f.Close()
	return true
}

func touchSession(sessionFile string) {
	_ = os.WriteFile(sessionFile, []byte(fmt.Sprintf("%d\n", time.Now().UnixNano())), 0644)
}

func appendPending(listFile, path string) {
	f, err := os.OpenFile(listFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(path + "\n")
}

func readPending(listFile string) []string {
	data, err := os.ReadFile(listFile)
	if err != nil {
		return nil
	}
	var out []string
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func uniqueDir(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	for i := 2; i < 1000; i++ {
		cand := fmt.Sprintf("%s_%d", path, i)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return fmt.Sprintf("%s_%d", path, time.Now().UnixNano())
}

func collectArgsFiles(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

func normalizeFiles(paths []string) []string {
	files := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, p := range paths {
		p = strings.TrimSpace(strings.Trim(p, `"'`))
		p = strings.TrimRight(p, "\r\n")
		if p == "" {
			continue
		}
		p, _ = filepath.Abs(p)
		key := strings.ToLower(p)
		if _, ok := seen[key]; ok {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		seen[key] = struct{}{}
		files = append(files, p)
	}
	return files
}

func isJpgExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return true
	default:
		return false
	}
}

func isCr3Ext(ext string) bool {
	return strings.ToLower(ext) == ".cr3"
}

func findJpgCr3Pairs(selected []string, dir string) map[string][]string {
	type bucket struct {
		jpg string
		cr3 string
	}

	selectedSet := map[string]struct{}{}
	for _, p := range selected {
		selectedSet[strings.ToLower(p)] = struct{}{}
	}

	byBase := map[string]*bucket{}
	ensure := func(base string) *bucket {
		if byBase[base] == nil {
			byBase[base] = &bucket{}
		}
		return byBase[base]
	}

	classify := func(p string) {
		ext := filepath.Ext(p)
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(p), ext))
		b := ensure(base)
		if isJpgExt(ext) {
			b.jpg = p
		} else if isCr3Ext(ext) {
			b.cr3 = p
		}
	}

	for _, p := range selected {
		classify(p)
	}

	entries, _ := os.ReadDir(dir)
	indexByBase := map[string]map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		base := strings.ToLower(strings.TrimSuffix(name, ext))
		full := filepath.Join(dir, name)
		if indexByBase[base] == nil {
			indexByBase[base] = map[string]string{}
		}
		if isJpgExt(ext) {
			indexByBase[base]["jpg"] = full
		} else if isCr3Ext(ext) {
			indexByBase[base]["cr3"] = full
		}
	}

	for base, b := range byBase {
		if info, ok := indexByBase[base]; ok {
			if b.jpg == "" {
				b.jpg = info["jpg"]
			}
			if b.cr3 == "" {
				b.cr3 = info["cr3"]
			}
		}
	}

	out := map[string][]string{}
	for base, b := range byBase {
		if b.jpg == "" || b.cr3 == "" {
			continue
		}
		_, jpgSel := selectedSet[strings.ToLower(b.jpg)]
		_, cr3Sel := selectedSet[strings.ToLower(b.cr3)]
		if !jpgSel && !cr3Sel {
			continue
		}
		out[base] = []string{b.jpg, b.cr3}
	}
	return out
}

func getExplorerSelection(hint string) []string {
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyFileWithProgress(src, dst string, onProgress func(int64)) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	buf := make([]byte, 1024*1024)
	var total int64
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			written, writeErr := out.Write(buf[:n])
			total += int64(written)
			if written > 0 && onProgress != nil {
				onProgress(int64(written))
			}
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return total, readErr
		}
	}
	return total, out.Close()
}

func sameFile(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return strings.EqualFold(a, b)
}

func messageBox(text, caption string) {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(caption)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40)
}
